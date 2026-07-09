# github action runner のセットアップ



## enp8s0となるインタフェースを作成　(VMホスト側の作業)
VMホスト側の作業として、enp8s0のインタフェースを作成して、子VMへVLANタグを通過させるルートを作る

`<address type='pci' domain='0x0000' bus='0x08' slot='0x00' function='0x0'/>`

設定結果の確認

~~~
# virsh dumpxml 対象vm名
＜中略＞
    <interface type='bridge'>
      <mac address='52:54:00:23:d6:db'/>
      <source network='ovs-network' portgroup='vlan-all' portid='ae4ed484-2f6f-4c1f-b8c8-431281a05902' bridge='ovsbr0'/>
      <vlan trunk='yes'>
        <tag id='1001'/>
        <tag id='1002'/>
      </vlan>
      <virtualport type='openvswitch'>
        <parameters interfaceid='f418235f-19b2-46bb-96ba-7bf2081c77a6'/>
      </virtualport>
      <target dev='vnet2'/>
      <model type='virtio'/>
      <alias name='net2'/>
      <address type='pci' domain='0x0000' bus='0x08' slot='0x00' function='0x0'/>
    </interface>
＜以下省略＞
~~~


## systemd-resolved.serviceを止める

```
# systemctl status systemd-resolved.service 
# systemctl stop systemd-resolved.service 
# systemctl disable systemd-resolved.service 
Removed /etc/systemd/system/multi-user.target.wants/systemd-resolved.service.
Removed /etc/systemd/system/dbus-org.freedesktop.resolve1.service.
```

## DNSリゾルバーの設定変更

```
# rm /etc/resolv.conf 
# vi /etc/resolv.conf
cat /etc/resolv.conf
# from Ansible template
#
nameserver 172.16.0.9
options edns0 trust-ad
search labo.local
```

設定確認
```
# dig www.yahoo.co.jp +short
edge12.g.yimg.jp.
183.79.219.252
```

## ubuntu アップデート

```
# apt-get update -y
# apt-get upgrade -y
# apt-get install -y git curl gcc make kpartx
# apt-get install -y virt-top virt-manager libvirt-dev libvirt-clients libvirt-daemon qemu-system-x86 openvswitch-switch ovn-central ovn-host libguestfs-tools libvirt-daemon-driver-lxc lxcfs bridge-utils genisoimage
```

Ubuntu Serverからでは、以下が入っていないので、以下を加える。
```
apt-get install -y linux-image-6.8.0-134-generic linux-modules-6.8.0-134-generic linux-modules-extra-6.8.0-134-generic linux-image-virtual etcd-client
```

## marmot のインストール

ws1 から ターゲットVMへ debパッケージを転送する。

```
ubuntu@ws1:~/marmot$ scp dist/marmot_v0.26.2_amd64.deb runner7:/tmp
marmot_v0.26.2_amd64.deb   
```

```
root@runner7:/tmp# apt install -y ./marmot_v0.26.2_amd64.deb 
```

marmot と etcd を起動停止、無効化する。

```
# systemctl stop marmot
# systemctl disable marmot
# systemctl stop etcd
# systemctl disable etcd
```


resolve.confが書き換えられるので、戻す

```
# vi /etc/resolv.conf
# cat /etc/resolv.conf
# generate by marmotd
nameserver 172.16.0.9
options edns0 trust-ad
search labo.local
```


`Open vSwitch` に設定を入れる。
「ovsbr0」に、先に設定したトランクポート「enp8s0」を繋ぐ。

~~~
# ovs-vsctl add-br ovsbr0
# ovs-vsctl add-port ovsbr0 enp8s0
# ovs-vsctl set port enp8s0 trunk=1001,1002
# ovs-vsctl show
~~~


ovs-network.xml
```
<network connections='4'>
  <name>ovs-network</name>
  <forward mode='bridge'/>
  <bridge name='ovsbr0'/>
  <virtualport type='openvswitch'/>
  <portgroup name='vlan-0001' default='yes'>
  </portgroup>
  <portgroup name='vlan-1001'>
    <vlan>
      <tag id='1001'/>
    </vlan>
  </portgroup>
  <portgroup name='vlan-1002'>
    <vlan>
      <tag id='1002'/>
    </vlan>
  </portgroup>
  <portgroup name='vlan-all'>
    <vlan trunk='yes'>
      <tag id='1001'/>
      <tag id='1002'/>
    </vlan>
  </portgroup>
</network>
```

上記のファイルを作成して、活性化する。

```
root@runner7:/tmp# cd /root
root@runner7:~# vi ovs-network.xml
root@runner7:~# virsh net-define ovs-network.xml 
root@runner7:~# virsh net-start ovs-network
root@runner7:~# virsh net-autostart ovs-network
```

runner上で ovs-network が起動したことを確認する。

```
root@runner7:~# virsh net-list
 Name          State    Autostart   Persistent
------------------------------------------------
 default       active   yes         yes
 host-bridge   active   yes         yes
 ovs-network   active   yes         yes
```


# LXCを有効化するために(不要)

```
systemctl stop libvirtd.service
systemctl disable libvirtd.service
virsh -c lxc:///system list
 Id   Name   State
--------------------

virsh list
 Id   Name   State
--------------------

# lxcfsを追加
systemctl start lxcfs
```


## LVMの設定

```
# lsblk
NAME   MAJ:MIN RM   SIZE RO TYPE MOUNTPOINTS
loop0    7:0    0    62M  1 loop /snap/core20/1587
loop1    7:1    0  63.5M  1 loop /snap/core20/2015
loop2    7:2    0  79.9M  1 loop /snap/lxd/22923
loop3    7:3    0 111.9M  1 loop /snap/lxd/24322
loop4    7:4    0    47M  1 loop /snap/snapd/16292
vda    252:0    0    16G  0 disk 
├─vda1 252:1    0     1M  0 part 
└─vda2 252:2    0    16G  0 part /
vdb    252:16   0   100G  0 disk 
vdc    252:32   0   100G  0 disk 
vdd    252:48   0   100G  0 disk 
```



```console
# mkfs.ext4 /dev/vdb
# blkid /dev/vdb
# vi /etc/fstab
# cat /etc/fstab |tail -n 1
UUID="3c6c38cd-52ba-4c49-8ef9-6a29472825db" /build ext4 defaults  0 0
# mkdir /build
# mount /build
# df -h
```

## PVの作成

```
# pvcreate /dev/vdc
  Physical volume "/dev/vdc" successfully created.
# pvcreate /dev/vdd
  Physical volume "/dev/vdd" successfully created.
```

## PGの作成

```
# vgcreate vg1 /dev/vdc
  Volume group "vg1" successfully created
# vgcreate vg2 /dev/vdd
  Volume group "vg2" successfully created
```

## PGの作成状態の確認

```
# vgs
  VG  #PV #LV #SN Attr   VSize    VFree   
  vg1   1   0   0 wz--n- <100.00g <100.00g
  vg2   1   0   0 wz--n- <100.00g <100.00g
```

## runner を /build 以下で動かすための準備

```
# cd /var/lib
# mv marmot /build/marmot-var
# ln -s /build/marmot-var marmot
# ls -la marmot
lrwxrwxrwx 1 root root 17 Jul  8 08:44 marmot -> /build/marmot-var
# cd marmot
# chown -R ubuntu:ubuntu volumes
# chown -R ubuntu:ubuntu jobs
# exit
$ cd /build
```

```
ubuntu@runner8:/build$ sudo chown ubuntu:ubuntu .
sudo: unable to resolve host runner8: Name or service not known
ubuntu@runner8:/build$ sudo vi /etc/host
```



## イメージテンプレート用のロジカルボリュームの作成(不要)

```
# lvcreate --name lv01 --size 16GB vg1
  Logical volume "lv01" created.
# lvs
  LV   VG  Attr       LSize  Pool Origin Data%  Meta%  Move Log Cpy%Sync Convert
  lv01 vg1 -wi-a----- 16.00g   
```

## NFSクライアントのインストール(不要)

```
# apt install nfs-common
```

## NFSサーバーのデータを利用するため、fstabに追加とNFSマウント（不要）

```
# vi /etc/fstab
# cat /etc/fstab
hmc2-nfs:/exports /nfs nfs defaults 0 0
# mkdir /nfs
# mount /nfs
# df -h
Filesystem                   Size  Used Avail Use% Mounted on
tmpfs                        1.6G  1.4M  1.6G   1% /run
/dev/vda2                     16G  6.8G  8.2G  46% /
tmpfs                        7.9G     0  7.9G   0% /dev/shm
tmpfs                        5.0M     0  5.0M   0% /run/lock
tmpfs                        1.6G  4.0K  1.6G   1% /run/user/1000
tmpfs                        7.9G     0  7.9G   0% /run/qemu
hmc-nfs:/exports/nfs/golang  110G   91G   14G  87% /nfs
```

## マウントポイントの追加(不用)

```
# mount -t nfs hmc2-nfs:/backup /mnt
# df -h
Filesystem                   Size  Used Avail Use% Mounted on
tmpfs                        1.6G  1.4M  1.6G   1% /run
/dev/vda2                     16G  6.8G  8.2G  46% /
tmpfs                        7.9G     0  7.9G   0% /dev/shm
tmpfs                        5.0M     0  5.0M   0% /run/lock
tmpfs                        1.6G  4.0K  1.6G   1% /run/user/1000
tmpfs                        7.9G     0  7.9G   0% /run/qemu
hmc-nfs:/exports/nfs/golang  110G   91G   14G  87% /nfs
hmc-nfs:/backup              110G   51G   54G  49% /mnt
```

## NFSサーバー上のディスクイメージをコピー（不要）

```
# dd if=/nfs/lv03.img of=/dev/vg1/lv01 bs=4294967296
0+9 records in
0+9 records out
17179869184 bytes (17 GB, 16 GiB) copied, 157.197 s, 109 MB/s
```

## /var を専用ストレージに移行(不要)

```
# vi /etc/fstab
# cat /etc/fstab
...
hmc-nfs:/exports/nfs/golang /nfs nfs defaults 0 0
/dev/vdb /var ext4 defaults 0 0

# mkfs.ext4 /dev/vdb
# cd /
# tar cvf /mnt/var.tar var
# mv var var.backup
# mkdir /var
# mount /var
# df -h
# tar xvf /nfs/var.tar 
# cd /
# rm -fr var.backup/
```


## ネットワークの設定

#### netplan でのブリッジの追加とIPアドレス設定の移動
bridge インターフェースを設定して、IPアドレスを付与して、インターフェースと結び付ける。
ブリッジのmacアドレスは、デフォルトでは同じ値となるため、
他の仮想マシンのブリッジIFのMACアドレスと衝突しないように、設定する。

```
root@runner7:/etc/netplan# ls
00-host-bridge.yaml  00-nic.yaml.marmot.bak
root@runner7:/etc/netplan# rm 00-nic.yaml.marmot.bak 
root@runner7:/etc/netplan# vi 00-host-bridge.yaml 
```

```xml
network:
    version: 2
    renderer: networkd
    ethernets:
        enp1s0:
            dhcp4: false
            dhcp6: false
        enp2s0:
            addresses:
                - 192.168.1.217/24
            dhcp4: false
            dhcp6: false
            routes:
                - to: default
                  via: 192.168.1.1
        enp7s0:
            addresses:
                - 172.16.0.217/24
            dhcp4: false
            dhcp6: false
            routes:
                - to: default
                  via: 172.16.0.1
    bridges:
        br0:
            interfaces:
                - enp1s0
            addresses:
                - 10.1.3.8/16
            macaddress: b6:bd:d8:c5:d3:17  # デフォルトでは同じMACアドレスとなるので、衝突しないように指定する。
            parameters:
               stp: true
            routes:
                - to: default
                  via: 10.1.0.1
            nameservers:
                addresses:
                    - 192.168.1.9
                search:
                    - labo.local
```

有効化する

```console
# netplan apply
```



#### virtのブリッジ設定 (marmotをインストールしていれば不要)
nfs ドライブ上にダウンしておいたXMLファイルから、仮想ネットワークを定義しておく。

```
root@runner2:/nfs# virsh net-define host-bridge.xml 
Network host-bridge defined from host-bridge.xml

root@runner2:/nfs# virsh net-define ovs-network.xml 
Network ovs-network defined from ovs-network.xml
```

---

## ネットワークの設定（ホスト側のハイパーバイザーの設定）(marmotをインストールしていれば不要)
  - https://github.com/takara9/marmot/blob/main/docs/network-setup-nested-vm.md#%E3%83%99%E3%82%A2%E3%83%A1%E3%82%BF%E3%83%AB%E3%81%AE%E3%83%8F%E3%82%A4%E3%83%91%E3%83%BC%E3%83%90%E3%82%A4%E3%82%B6%E3%83%BC%E5%81%B4

## ネットワークの設定（ランナー側の設定）(marmotをインストールしていれば不要)
  - https://github.com/takara9/marmot/blob/main/docs/HOWTO-install-marmot.md#5-%E3%83%8D%E3%83%83%E3%83%88%E3%83%AF%E3%83%BC%E3%82%AF%E3%81%AE%E8%A8%AD%E5%AE%9A
  - https://github.com/takara9/marmot/blob/main/docs/network-setup-nested-vm.md#%E3%83%99%E3%82%A2%E3%83%A1%E3%82%BF%E3%83%AB%E3%81%AE%E3%83%8F%E3%82%A4%E3%83%91%E3%83%BC%E3%83%90%E3%82%A4%E3%82%B6%E3%83%BC%E5%81%B4


先に作成した仮想ネットワークを有効化する。

virsh net-start host-bridge
virsh net-autostart host-bridge
virsh net-start ovs-network
virsh net-autostart ovs-network
virsh net-list

```
root@runner1:/etc/netplan# virsh net-list
 Name          State    Autostart   Persistent
------------------------------------------------
 default       active   yes         yes  　　　　NATで内部から外部への通信可能
 host-bridge   active   yes         yes          ホストのNICにIPアドレスを確保して外部からのアクセスを可能
 ovs-network   active   yes         yes          L2スイッチと連携してトランクVLAN設定を実施
```

---


## Dockerのインストール
  - インストール https://docs.docker.com/engine/install/ubuntu/
  - 一般ユーザーが起動するための設定 https://docs.docker.com/engine/install/linux-postinstall

## Go言語のインストール
  - ダウンロードとインストール https://go.dev/doc/install
　- パスの設定 https://github.com/takara9/marmot/blob/main/docs/HOWTO-install-golang.md
  - rootのホームにも設定すること


## Minioクライアント
  - wget https://dl.min.io/client/mc/release/linux-amd64/mc
  - chmod +x mc
  - sudo mv mc /usr/local/bin/
  - mc --help  


## systemdから起動できるように設定

```
# cd /var
# mkdir actions-runner
# chown ubuntu:ubuntu -R actions-runner
```


## GitHub Action runnerのインストール
  - https://github.com/takara9/marmot/settings/actions/runners


```console
ubuntu@runner7:~$ cd /build
ubuntu@runner7:/build$ ls -al
total 28
drwxr-xr-x  4 root root  4096 Jul  8 23:03 .
drwxr-xr-x 23 root root  4096 Jul  8 23:01 ..
drwx------  2 root root 16384 Jul  8 22:59 lost+found
drwxrwxr-x  7 root root  4096 Jul  8 22:51 marmot-var
ubuntu@runner7:/build$ sudo chown ubuntu:ubuntu .
```

```console
ubuntu@runner7:/build$ mkdir actions-runner && cd actions-runner
ubuntu@runner7:/build/actions-runner$ curl -o actions-runner-linux-x64-2.335.1.tar.gz -L https://github.com/actions/runner/releases/download/v2.335.1/actions-runner-linux-x64-2.335.1.tar.gz
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
  0     0    0     0    0     0      0      0 --:--:-- --:--:-- --:--:--     0
100  215M  100  215M    0     0  71.9M      0  0:00:02  0:00:02 --:--:-- 73.9M
ubuntu@runner7:/build/actions-runner$ echo "4ef2f25285f0ae4477f1fe1e346db76d2f3ebf03824e2ddd1973a2819bf6c8cf  actions-runner-linux-x64-2.335.1.tar.gz" | shasum -a 256 -c
actions-runner-linux-x64-2.335.1.tar.gz: OK
ubuntu@runner7:/build/actions-runner$ tar xzf ./actions-runner-linux-x64-2.335.1.tar.gz
```


```console
ubuntu@runner7:/build/actions-runner$ ./config.sh --url https://github.com/takara9/marmot --token *****************************

--------------------------------------------------------------------------------
|        ____ _ _   _   _       _          _        _   _                      |
|       / ___(_) |_| | | |_   _| |__      / \   ___| |_(_) ___  _ __  ___      |
|      | |  _| | __| |_| | | | | '_ \    / _ \ / __| __| |/ _ \| '_ \/ __|     |
|      | |_| | | |_|  _  | |_| | |_) |  / ___ \ (__| |_| | (_) | | | \__ \     |
|       \____|_|\__|_| |_|\__,_|_.__/  /_/   \_\___|\__|_|\___/|_| |_|___/     |
|                                                                              |
|                       Self-hosted runner registration                        |
|                                                                              |
--------------------------------------------------------------------------------

# Authentication


√ Connected to GitHub

# Runner Registration

Enter the name of the runner group to add this runner to: [press Enter for Default] 

Enter the name of runner: [press Enter for runner7] 

This runner will have the following labels: 'self-hosted', 'Linux', 'X64' 
Enter any additional labels (ex. label-1,label-2): [press Enter to skip] any

√ Runner successfully added

# Runner settings

Enter name of work folder: [press Enter for _work] 

√ Settings Saved.
```


```
ubuntu@runner7:/build/actions-runner$ ./run.sh 

√ Connected to GitHub

Current runner version: '2.335.1'
2026-07-08 23:21:43Z: Listening for Jobs
^CExiting...
Runner listener exit with 0 return code, stop the service, no retry needed.
Exiting runner...


ubuntu@runner7:/build/actions-runner$ sudo ./svc.sh install
Creating launch runner in /etc/systemd/system/actions.runner.takara9-marmot.runner7.service
Run as user: ubuntu
Run as uid: 1000
gid: 1000
Created symlink /etc/systemd/system/multi-user.target.wants/actions.runner.takara9-marmot.runner7.service → /etc/systemd/system/actions.runner.takara9-marmot.runner7.service.


ubuntu@runner7:/build/actions-runner$ sudo ./svc.sh start

/etc/systemd/system/actions.runner.takara9-marmot.runner7.service
● actions.runner.takara9-marmot.runner7.service - GitHub Actions Runner (takara9-marmot.runner7)
     Loaded: loaded (/etc/systemd/system/actions.runner.takara9-marmot.runner7.service; enabled; preset: enabled)
     Active: active (running) since Wed 2026-07-08 23:22:06 UTC; 11ms ago
   Main PID: 28187 (runsvc.sh)
      Tasks: 2 (limit: 14307)
     Memory: 1.5M (peak: 1.5M)
        CPU: 6ms
     CGroup: /system.slice/actions.runner.takara9-marmot.runner7.service
             ├─28187 /bin/bash /build/actions-runner/runsvc.sh
             └─28190 ./externals/node20/bin/node ./bin/RunnerService.js

Jul 08 23:22:06 runner7 systemd[1]: Started actions.runner.takara9-marmot.runner7.service - GitHub Actions Runner (takara9-….runner7).
Jul 08 23:22:06 runner7 runsvc.sh[28187]: .path=/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sb…/snap/bin
Hint: Some lines were ellipsized, use -l to show in full.
```

以上
