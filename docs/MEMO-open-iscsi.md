# ストレージシステムの検証


```
root@marmot-stg:~# dpkg -l |grep open-iscsi
ii  open-iscsi                      2.1.9-3ubuntu5.4                        amd64        iSCSI initiator tools
```

```
apt update
apt install -y targetcli-fb
systemctl enable --now rtslib-fb-targetctl
```



## ボリュームの作成

```
pvcreate /dev/vdb
vgcreate vg2 /dev/vdb
```

```
root@marmot-stg:/etc/iscsi# vgs
  VG  #PV #LV #SN Attr   VSize    VFree   
  vg2   1   0   0 wz--n- <200.00g <200.00g
```

```
lvcreate --name lv01 --size 4GB vg2
lvcreate --name lv02 --size 4GB vg2
```

```
root@marmot-stg:~# lvs
  LV   VG  Attr       LSize Pool Origin Data%  Meta%  Move Log Cpy%Sync Convert
  lv01 vg2 -wi-ao---- 4.00g                                                    
  lv02 vg2 -wi-ao---- 5.00g  
```

## iSCSIターゲットの設定  LVM を iSCSI LUN として公開（例: /dev/vg2/lv01）

### ① バックストア作成
```
targetcli /backstores/block create user1 /dev/vg2/lv01
targetcli /backstores/block create user2 /dev/vg2/lv02
```

### ② iSCSI ターゲット作成（ユーザーごと）
```
targetcli /iscsi create iqn.2024-01.com.marmot:target-user1
targetcli /iscsi create iqn.2024-01.com.marmot:target-user2
```

### ③ LUN 割り当て
```
targetcli /iscsi/iqn.2024-01.com.marmot:target-user1/tpg1/luns create /backstores/block/user1
targetcli /iscsi/iqn.2024-01.com.marmot:target-user2/tpg1/luns create /backstores/block/user2
```

### ④ ACL 設定（イニシエーター IQN の紐付け）
```
targetcli /iscsi/iqn.2024-01.com.marmot:target-user1/tpg1/acls create iqn.2024-01.com.marmot:client-user1
targetcli /iscsi/iqn.2024-01.com.marmot:target-user2/tpg1/acls create iqn.2024-01.com.marmot:client-user2
```

### ⑤ 設定を保存（再起動後も有効）

```
sudo targetcli saveconfig
```


### 設定全体を確認
```
sudo targetcli ls
ss -lntp | grep 3260
LISTEN 0      256          0.0.0.0:3260      0.0.0.0:*   
```



## イニシエーター側の設定

/etc/iscsi/initiatorname.iscsi の編集
```
root@client-1:/etc/iscsi# cat initiatorname.iscsi 
InitiatorName=iqn.2024-01.com.marmot:client-user1
```

iSCSIサーバーで、ターゲットを探索
```
iscsiadm -m discovery -t sendtargets -p 192.168.1.210
```

ターゲットを指定してログイン
```
iscsiadm -m node -T iqn.2024-01.com.marmot:target-user1 -p 192.168.1.210 --login
```

デバイスを取り出し
```
# dev=$(ls -l /dev/disk/by-path/ | grep "target-user1" | awk '{print "/dev/" $NF}' | sed 's|/dev/\.\./\.\./|/dev/|')
# echo $dev
/dev/sda
# mkfs.ext4 ${dev}
```

### Linuxへの設定


UUIDの取り出し
```
UUID=$(blkid -s UUID -o value ${dev}) && echo $UUID
```


UUIDとマウントポイントを/etc/fstabの設定
```
LABEL=cloudimg-rootfs	/	 ext4	discard,commit=30,errors=remount-ro	0 1
LABEL=BOOT	/boot	ext4	defaults	0 2
LABEL=UEFI	/boot/efi	vfat	umask=0077	0 1
UUID=254bda08-e32f-46c1-af80-307892707dfe  /mnt  ext4  _netdev,auto,nofail  0 0
```

```
mount /mnt
```

### 自動マウントのための設定
```
iscsiadm -m node -T iqn.2024-01.com.marmot:target-user1 -p 192.168.1.210 --op update -n node.startup -v automatic
systemctl enable open-iscsi
systemctl enable iscsid
```





## 設計にあたっての確認事項

- 1ユーザーに複数のデバイスを割当た場合、クライアント側から、どの様に見えるか？　デバイス名とかのこと。
- libvirt の XMLから、ディスクをマウントする方法


複数のLUNがある場合は、以下のとおり
```
root@client-1:/etc/iscsi# ls -l /dev/disk/by-path/ | grep "target-disks"
lrwxrwxrwx 1 root root  9 May  3 09:21 ip-172.16.0.210:3260-iscsi-iqn.2024-01.com.marmot:target-disks-lun-0 -> ../../sda
lrwxrwxrwx 1 root root  9 May  3 09:21 ip-172.16.0.210:3260-iscsi-iqn.2024-01.com.marmot:target-disks-lun-1 -> ../../sdb
lrwxrwxrwx 1 root root  9 May  3 09:21 ip-172.16.0.210:3260-iscsi-iqn.2024-01.com.marmot:target-disks-lun-2 -> ../../sdc

root@client-1:/etc/iscsi# ls -l /dev/disk/by-path/ | grep "target-disks" | awk '{print "/dev/" $NF}'
/dev/../../sda
/dev/../../sdb
/dev/../../sdc


mkfs.ext4 /dev/sda
mkfs.ext4 /dev/sdb
mkfs.ext4 /dev/sdc

root@client-1:/etc/iscsi# blkid /dev/sda
/dev/sda: UUID="b3dc8c9a-d177-441d-85ff-ed5017a06fe2" BLOCK_SIZE="4096" TYPE="ext4"
root@client-1:/etc/iscsi# blkid /dev/sdb
/dev/sdb: UUID="f48ae856-8b65-4798-ab87-6b7f1c0f5c14" BLOCK_SIZE="4096" TYPE="ext4"
root@client-1:/etc/iscsi# blkid /dev/sdc
/dev/sdc: UUID="462381e9-cfb6-4914-a8e8-8139f0f85b27" BLOCK_SIZE="4096" TYPE="ext4"
```


## hv2からマウントする
ディスカバリーして、ターゲットを指定してログインすれば、ディスクが見える。

```
root@hv2:/etc/iscsi# ls -l /dev/disk/by-path
total 0
lrwxrwxrwx 1 root root  9 May  3 20:43 ip-192.168.1.210:3260-iscsi-iqn.2024-01.com.marmot:target-user2-lun-0 -> ../../sdd
lrwxrwxrwx 1 root root  9 May  3 04:31 pci-0000:00:17.0-ata-2 -> ../../sda
lrwxrwxrwx 1 root root  9 May  3 04:31 pci-0000:00:17.0-ata-2.0 -> ../../sda
lrwxrwxrwx 1 root root 10 May  3 04:31 pci-0000:00:17.0-ata-2.0-part1 -> ../../sda1
lrwxrwxrwx 1 root root 10 May  3 04:31 pci-0000:00:17.0-ata-2-part1 -> ../../sda1
lrwxrwxrwx 1 root root  9 May  3 04:31 pci-0000:00:17.0-ata-3 -> ../../sdb
lrwxrwxrwx 1 root root  9 May  3 04:31 pci-0000:00:17.0-ata-3.0 -> ../../sdb
lrwxrwxrwx 1 root root  9 May  3 04:31 pci-0000:00:17.0-ata-4 -> ../../sdc
lrwxrwxrwx 1 root root  9 May  3 04:31 pci-0000:00:17.0-ata-4.0 -> ../../sdc
lrwxrwxrwx 1 root root 13 May  3 04:30 pci-0000:02:00.0-nvme-1 -> ../../nvme0n1
lrwxrwxrwx 1 root root 15 May  3 04:30 pci-0000:02:00.0-nvme-1-part1 -> ../../nvme0n1p1
lrwxrwxrwx 1 root root 15 May  3 04:30 pci-0000:02:00.0-nvme-1-part2 -> ../../nvme0n1p2
```


## libvirt のXML設定でログインする方法

libvirtのXMLで、ログインする場合は、CHAPを設定が必須。設定が無いとリードオンリーになる。
CHAPとは、Challenge Handshake Authentication Protocol の略で、ネットワーク接続時の認証プロトコルです。


### ハイパーバイザーホスト hv2 の イニシエーター名
```
root@hv2:/home/ubuntu# cat /etc/iscsi/initiatorname.iscsi 
## DO NOT EDIT OR REMOVE THIS FILE!
## If you remove this file, the iSCSI daemon will not start.
## If you change the InitiatorName, existing access control lists
## may reject this initiator.  The InitiatorName must be unique
## for each iSCSI initiator.  Do NOT duplicate iSCSI InitiatorNames.
InitiatorName=iqn.2004-10.com.ubuntu:01:c1b5e3a5db
```

### ターゲット側の設定 (marmot-stg)

専用ストレージを作成

```
lvcreate --name lv04 --size 4GB vg2
```

ターゲットと論理ユニット、アクセスを許可するイニシエーターを設定
```
targetcli /backstores/block create disk4 /dev/vg2/lv04
targetcli /iscsi create iqn.2024-01.com.marmot:target-user2
targetcli /iscsi/iqn.2024-01.com.marmot:target-user2/tpg1/luns create /backstores/block/disk4
targetcli /iscsi/iqn.2024-01.com.marmot:target-user2/tpg1/acls create iqn.2004-10.com.ubuntu:01:c1b5e3a5db
targetcli saveconfig
```


CHAP認証を有効化、認証ユーザーとパスワードを設定
```
targetcli /iscsi/iqn.2024-01.com.marmot:target-user2/tpg1   set attribute authentication=1
targetcli /iscsi/iqn.2024-01.com.marmot:target-user2/tpg1   set attribute generate_node_acls=0
targetcli /iscsi/iqn.2024-01.com.marmot:target-user2/tpg1/acls/iqn.2004-10.com.ubuntu:01:c1b5e3a5db   set auth userid=user2
targetcli /iscsi/iqn.2024-01.com.marmot:target-user2/tpg1/acls/iqn.2004-10.com.ubuntu:01:c1b5e3a5db   set auth password=PassUser2123
targetcli saveconfig
```

### イニシエーター側の設定 (marmot-client-2)

ディスク設定部分のXML設定
- 認証のためのシークレットを指定
- iSCSIのターゲット名＋LUN番号をセット
- iSCSIターゲットのIPドレスポート
- 自己のiSCSIイニシエーター名をセット

```
    <disk type='network' device='disk'>
      <driver name='qemu' type='raw' cache='none' io='native'/>
      <auth username='user2'>
        <secret type='iscsi' usage='iscsi-user2'/>
      </auth>
      <source protocol='iscsi' name='iqn.2024-01.com.marmot:target-user2/0'>
        <host name='192.168.1.210' port='3260'/>
        <initiator>
          <iqn name='iqn.2004-10.com.ubuntu:01:c1b5e3a5db'/>
        </initiator>
      </source>
      <target dev='vdb' bus='virtio'/>
      <address type='pci' domain='0x0000' bus='0x07' slot='0x00' function='0x0'/>
    </disk>
```


### 認証のための設定として、libvirtにシークレットを作成する方法


iscsi-secret-user2.xml
```
<secret ephemeral='no' private='yes'>
  <usage type='iscsi'>
    <target>iscsi-user2</target>
  </usage>
</secret>
```

シークレットの作成と確認
```
root@hv2:/home/ubuntu# virsh secret-define iscsi-secret-user3.xml 
Secret 7cceac19-51ec-4303-b6e7-1374458ffba7 created

root@hv2:/home/ubuntu# virsh secret-list
 UUID                                   Usage
-----------------------------------------------------------
 26d9b905-4191-42be-bcac-02b09ddd71d2   iscsi iscsi-user2
 7cceac19-51ec-4303-b6e7-1374458ffba7   iscsi iscsi-user3

root@hv2:/home/ubuntu# 
```

UUIDを取り出す方法
```
TGT=iscsi-user2
UUID=$(virsh secret-list | grep ${TGT} | awk '{print $1}')
```

パスワードを BASE64 でエンコードする
```
AA=$(echo -n 'PassUser2123' | base64)
printf ${AA} | sudo virsh secret-set-value ${UUID} --file /dev/stdin
```

### マウントと確認

```
root@client-2:~# lsblk
NAME    MAJ:MIN RM  SIZE RO TYPE MOUNTPOINTS
sr0      11:0    1  368K  0 rom  
vda     253:0    0   16G  0 disk 
├─vda1  253:1    0   15G  0 part /
├─vda14 253:14   0    4M  0 part 
├─vda15 253:15   0  106M  0 part /boot/efi
└─vda16 259:0    0  913M  0 part /boot
vdb     253:16   0    4G  0 disk 
root@client-2:~# mkfs.ext4 /dev/vdb
mke2fs 1.47.0 (5-Feb-2023)
/dev/vdb contains a ext4 file system
	last mounted on Sun May  3 12:02:58 2026
Proceed anyway? (y,N) y
Discarding device blocks: done                            
Creating filesystem with 1048576 4k blocks and 262144 inodes
Filesystem UUID: fc15618a-0487-4c01-8e99-25131c6064ef
Superblock backups stored on blocks: 
	32768, 98304, 163840, 229376, 294912, 819200, 884736

Allocating group tables: done                            
Writing inode tables: done                            
Creating journal (16384 blocks): done
Writing superblocks and filesystem accounting information: done 

root@client-2:~# mount -t ext4 /dev/vdb /mnt
root@client-2:~# df
Filesystem     1K-blocks    Used Available Use% Mounted on
tmpfs             201488    1072    200416   1% /run
/dev/vda1       15159300 1765548  13377368  12% /
tmpfs            1007436       0   1007436   0% /dev/shm
tmpfs               5120       0      5120   0% /run/lock
/dev/vda16        901520   64732    773660   8% /boot
/dev/vda15        106832    6250    100582   6% /boot/efi
tmpfs             201484      12    201472   1% /run/user/0
/dev/vdb         4046560      24   3820440   1% /mnt
root@client-2:~# 
```


## 性能試験
ローカルとiSCSIのデバイスへの書き込み、読み取りのテストを実施した。
この動作条件では、iSCSI経由は 約1/10 程度に性能は劣化する

iSCSIデバイスへの書き込み
```
root@client-2:/mnt# dd if=/dev/zero of=/mnt/testfile bs=1M count=1024 oflag=direct
1024+0 records in
1024+0 records out
1073741824 bytes (1.1 GB, 1.0 GiB) copied, 35.1909 s, 30.5 MB/s
```

iSCSIデバイスからの読み込み
```
root@client-2:/mnt# dd if=/mnt/testfile of=/dev/null bs=1M iflag=direct
1024+0 records in
1024+0 records out
1073741824 bytes (1.1 GB, 1.0 GiB) copied, 24.8919 s, 43.1 MB/s
```

ローカルディスクへの書き込みと読み取り
```
root@client-2:~# dd if=/dev/zero of=/tmp/testfile bs=1M count=1024 oflag=direct
1024+0 records in
1024+0 records out
1073741824 bytes (1.1 GB, 1.0 GiB) copied, 2.6975 s, 398 MB/s

root@client-2:~# dd if=/tmp/testfile of=/dev/null bs=1M iflag=direct
1024+0 records in
1024+0 records out
1073741824 bytes (1.1 GB, 1.0 GiB) copied, 2.15931 s, 497 MB/s
```

