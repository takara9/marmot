# marmot ホスト上に、marmot clusterを構築する手順

この文章は、Nested VM でクラスタ環境を構築するときのメモです。


## 仮想マシンのデプロイ

このdocsの下にある marmot-cluster-on-marmot の３つのマニフェストを適用する。

```console
cd marmot-cluster-on-marmot
mactl server create -f marmot-cluster-1.yaml
mactl server create -f marmot-cluster-2.yaml
mactl server create -f marmot-cluster-3.yaml
```

## /varの拡大

marmot1にログインする。

```console
ubuntu@marmot1:~$ sudo -s
root@marmot1:/home/ubuntu# lsblk
NAME    MAJ:MIN RM  SIZE RO TYPE MOUNTPOINTS
sr0      11:0    1  368K  0 rom  
vda     253:0    0   16G  0 disk 
├─vda1  253:1    0   15G  0 part /
├─vda14 253:14   0    4M  0 part 
├─vda15 253:15   0  106M  0 part /boot/efi
└─vda16 259:0    0  913M  0 part /boot
vdb     253:16   0  200G  0 disk 
vdc     253:32   0  100G  0 disk 
vdd     253:48   0  100G  0 disk 
```

ファイルシステムを作成する

```console
root@marmot1:/home/ubuntu# mkfs.ext4 /dev/vdb
root@marmot1:/home/ubuntu# cd /
root@marmot1:/# tar cvf var.tar /var
root@marmot1:/# blkid /dev/vdb
/dev/vdb: UUID="68210548-e296-4c70-898e-7502ed42ff88" BLOCK_SIZE="4096" TYPE="ext4"
```

マウントポイントを設定する

```console
root@marmot1:/# vi /etc/fstab
root@marmot1:/# cat /etc/fstab
LABEL=cloudimg-rootfs	/	 ext4	discard,commit=30,errors=remount-ro	0 1
LABEL=BOOT	/boot	ext4	defaults	0 2
LABEL=UEFI	/boot/efi	vfat	umask=0077	0 1
UUID=68210548-e296-4c70-898e-7502ed42ff88 /var ext4 defaults  0 1
root@marmot1:/# mount /var
mount: (hint) your fstab has been modified, but systemd still uses
       the old version; use 'systemctl daemon-reload' to reload.
```

varの内容を復元する

```console
root@marmot1:/# tar xvf var.tar 
```

## marmot のインストール

開発マシンから、インストールパッケージを転送する

```console
ubuntu@ws1:~/marmot/dist$ scp marmot_v0.20.0_amd64.deb 192.168.1.201:/tmp
```

インストール前に、必ず`apt-get update`で、リポジトリの情報を更新しておくこと。

```console
ubuntu@marmot1:/tmp$ sudo apt-get update
ubuntu@marmot1:/tmp$ sudo apt install -y ./marmot_v0.20.0_amd64.deb 
```

インストールが終わったら、marmot と etcd を停止させる。

```console
root@marmot1:/tmp# systemctl stop marmot
root@marmot1:/tmp# systemctl stop etcd
```

クラスタの他メンバーと etcd を共有できるように、クラスタネットワークのIPアドレスに
ポートを開くように設定する。

```console
root@marmot1:/tmp# vi  /usr/lib/systemd/system/etcd.service
root@marmot1:/tmp# grep DAEMON_ARGS=  /usr/lib/systemd/system/etcd.service
Environment=DAEMON_ARGS="--listen-client-urls http://172.16.0.201:2379 --advertise-client-urls http://172.16.0.201:2379"
```

変更した設定を再読込してetcdを起動する。

```console
root@marmot1:/tmp# systemctl daemon-reload
root@marmot1:/tmp# systemctl start etcd
root@marmot1:/tmp# systemctl status etcd
● etcd.service - etcd - highly-available key value store
     Loaded: loaded (/usr/lib/systemd/system/etcd.service; enabled; preset: enabled)
     Active: active (running) since Wed 2026-06-10 02:36:24 UTC; 45s ago
       Docs: https://etcd.io/docs
```


## marmot のクラスタ設定

以下３点を修正する。

- etcdのURL etcd_url
- marmotのDNSのリッスンアドレス 192.168.1.201:53
- DNSのアップストリーム 192.168.1.9:53

```console
root@marmot1:/tmp# cd /etc/marmot/
root@marmot1:/etc/marmot# vi marmotd.json 
root@marmot1:/etc/marmot# head marmotd.json 
{
  "node_name": "marmot1",
  "etcd_url": "http://172.16.0.201:2379",
  "api_listen_addr": "0.0.0.0:8750",
  "dns_listen_addr": "192.168.1.201:53",
  "dns_upstream": "192.168.1.9:53",
  "dns_upstream_allow_cidrs": [
    "192.168.1.0/24"
  ],
  "default_underlay_interface": "",
```

設定が完了したら、marmot を起動する。

```console
root@marmot1:/etc/marmot# systemctl start marmot
root@marmot1:/etc/marmot# systemctl status marmot |head -n 3
● marmot.service - marmot - vm cluster service
     Loaded: loaded (/usr/lib/systemd/system/marmot.service; enabled; preset: enabled)
     Active: active (running) since Wed 2026-06-10 02:40:01 UTC; 25s ago
```


## クラスタメンバーの登録

基本は同じである。

- marmotのインストール
- /var の拡張
- etdd の起動停止
- /etc/marmot/marmotd.json を クラスタ用に編集

設定実施の間 marmot を停止、etdの起動を停止。

```console
root@marmot2:/tmp# systemctl stop marmot
root@marmot2:/tmp# systemctl stop etcd
root@marmot2:/tmp# systemctl disable etcd
Synchronizing state of etcd.service with SysV service script with /usr/lib/systemd/systemd-sysv-install.
Executing: /usr/lib/systemd/systemd-sysv-install disable etcd
Removed "/etc/systemd/system/etcd2.service".
Removed "/etc/systemd/system/multi-user.target.wants/etcd.service".
```

## marmotd.json の編集

- etcd_url マスターノードに向ける
- dns_listen_addr 自ノードのパブリック側IPアドレスをセット
- dns_upstream 上位のDNSサーバーへ向ける

```console
root@marmot2:/tmp# vi /etc/marmot/marmotd.json 
root@marmot2:/tmp# head /etc/marmot/marmotd.json 
{
  "node_name": "marmot2",
  "etcd_url": "http://172.16.0.201:2379",
  "api_listen_addr": "0.0.0.0:8750",
  "dns_listen_addr": "192.168.1.202:53",
  "dns_upstream": "192.168.1.9:53",
  "dns_upstream_allow_cidrs": [
    "192.168.1.0/24"
  ],
  "default_underlay_interface": "",
root@marmot2:/tmp# 
```

## marmot の起動
コマンドラインから、起動して、起動が成功したことを確認しておく。

```console
root@marmot2:/tmp# systemctl start marmot
root@marmot2:/tmp# systemctl status marmot |head -n 3
● marmot.service - marmot - vm cluster service
     Loaded: loaded (/usr/lib/systemd/system/marmot.service; enabled; preset: enabled)
     Active: active (running) since Wed 2026-06-10 02:51:19 UTC; 11s ago
```

## イメージの再作成

marmot クラスタは、各メンバーが起動用の qcow2 イメージを持っている。
クラスタ構成後に、image を再読込させて、仮想サーバーの起動に備える必要がある。

- mactl get img 状態確認
- mactl del image NAME ダウンロードに失敗したイメージを削除
- marmot1 で systemctl restart marmot で再起動
- mactl get img で初期化の進行を確認する


