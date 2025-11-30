# github action runner のセットアップ

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
# apt-get install git curl gcc make kpartx
# apt-get install virt-top  virt-manager libvirt-dev libvirt-clients libvirt-daemon qemu-kvm qemu openvswitch-switch openvswitch-common openvswitch-doc
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

## イメージテンプレート用のロジカルボリュームの作成

```
# lvcreate --name lv01 --size 16GB vg1
  Logical volume "lv01" created.
# lvs
  LV   VG  Attr       LSize  Pool Origin Data%  Meta%  Move Log Cpy%Sync Convert
  lv01 vg1 -wi-a----- 16.00g   
```

## NFSクライアントのインストール

```
# apt install nfs-common
```

## NFSサーバーのデータを利用するため、fstabに追加とNFSマウント

```
# vi /etc/fstab
# cat /etc/fstab
hmc-nfs:/exports/nfs/golang /nfs nfs defaults 0 0
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

## マウントポイントの追加

```
# mount -t nfs hmc-nfs:/backup /mnt
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

## NFSサーバー上のディスクイメージをコピー

```
# dd if=/mnt/lv03.img of=/dev/vg1/lv01 bs=4294967296
0+9 records in
0+9 records out
17179869184 bytes (17 GB, 16 GiB) copied, 157.197 s, 109 MB/s
```

## /var を専用ストレージに移行

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
# tar xvf /mnt/var.tar 
# cd /
# rm -fr var.backup/
```


## ネットワークの設定（ホスト側のハイパーバイザーの設定）

  - https://github.com/takara9/marmot/blob/main/docs/network-setup-nested-vm.md#%E3%83%99%E3%82%A2%E3%83%A1%E3%82%BF%E3%83%AB%E3%81%AE%E3%83%8F%E3%82%A4%E3%83%91%E3%83%BC%E3%83%90%E3%82%A4%E3%82%B6%E3%83%BC%E5%81%B4

## ネットワークの設定（ランナー側の設定）
  - https://github.com/takara9/marmot/blob/main/docs/network-setup-nested-vm.md#open-vswitch%E3%81%AE%E8%A8%AD%E5%AE%9A
  - https://github.com/takara9/marmot/blob/main/docs/network-setup-nested-vm.md#%E3%83%99%E3%82%A2%E3%83%A1%E3%82%BF%E3%83%AB%E3%81%AE%E3%83%8F%E3%82%A4%E3%83%91%E3%83%BC%E3%83%90%E3%82%A4%E3%82%B6%E3%83%BC%E5%81%B4

## Dockerのインストール
  - インストール https://docs.docker.com/engine/install/ubuntu/
  - 一般ユーザーが起動するための設定 https://docs.docker.com/engine/install/linux-postinstall

## Go言語のインストール
  - ダウンロードとインストール https://go.dev/doc/install
　- パスの設定 https://github.com/takara9/marmot/blob/main/docs/HOWTO-install-golang.md
  - rootのホームにも設定すること


## systemdから起動できるように設定

```
# cd /var
# mkdir actions-runner
# chown ubuntu:ubuntu -R actions-runner
```


## GitHub Action runnerのインストール
  - https://github.com/takara9/marmot/settings/actions/runners


