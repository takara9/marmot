# ハイパーバイザーノードの構成

##　NICの追加
仮想サーバー用のI/F、および、ハイパーバイザー上の仮想サーバーの連携のために、２ポートのNICを追加する。
購入したNICのアマゾンのリンク
  - https://www.amazon.co.jp/gp/product/B07B4RL9QJ/ref=ppx_yo_dt_b_search_asin_title?ie=UTF8&th=1


## Ubuntu 22.04のインストール
Ubuntu の USBブータブルメディアの作り方を参考にして、USBメモリを使ってインストールする
  - https://help.ubuntu.com/community/Installation/FromUSBStick
  - https://ubuntu.com/tutorials/create-a-usb-stick-on-ubuntu#1-overview

作業用ユーザーとして、ubuntuを作成しておく。

次のパッケージをインストールして、PCからsshでログインできるようにすると便利です。
```
apt install openssh-server
```


## 必要パッケージ
marmot を動かすために、以下のパッケージをインストールする。

```
$ sudo apt-get update -y
$ sudo apt-get install git curl gcc make kpartx
$ sudo apt-get install virt-top  virt-manager libvirt-dev libvirt-clients libvirt-daemon qemu-kvm qemu openvswitch-switch openvswitch-common openvswitch-doc
```


## NICの設定

### I/F名とポートの確認
マザーボードのネットワークI/Fを管理用LANに接続して、固定のIPアドレスを設定する。

NICの名前をリストする
```
# for DEV in `find /sys/devices -name net | grep -v virtual`; do ls $DEV/; done
enp4s0f1
enp4s0f0
enp42s0
wlo1
enp5s0
```

ポートに、HUBに繋がったLANケーブルを刺して、リンクアップを確認する。

```
# ip l
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
2: enp5s0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP mode DEFAULT group default qlen 1000
    link/ether 00:e0:4c:68:28:89 brd ff:ff:ff:ff:ff:ff
3: enp42s0: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc fq_codel state DOWN mode DEFAULT group default qlen 1000
    link/ether d8:43:ae:93:a4:09 brd ff:ff:ff:ff:ff:ff
4: enp4s0f0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP mode DEFAULT group default qlen 1000
    link/ether 00:15:17:a9:fc:f4 brd ff:ff:ff:ff:ff:ff
5: enp4s0f1: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP mode DEFAULT group default qlen 1000
    link/ether 00:15:17:a9:fc:f5 brd ff:ff:ff:ff:ff:ff
6: wlo1: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP mode DORMANT group default qlen 1000
    link/ether b0:35:9f:b2:85:90 brd ff:ff:ff:ff:ff:ff
    altname wlp41s0
```

### NICの役割を決める

ポートの役割を決める
  - enp5s0 を 管理用
  - enp4s0f0, enp4s0f1 の二つを 仮想マシン用
  - その他は不活性化

### networkdを設定して、ポートを活性化する

上記で決めたNICポートについて、/etc/netplan/01-network-manager-all.yaml を編集する
管理用のIPアドレスは、10.1.0.10 で最後の数値に hvのノード番号を足した値とする。

以下のケースでは、hv1の設定となる。

```
network:
  version: 2
  renderer: networkd
  ethernets:
    # Managment
    enp5s0:
      addresses:
      - 10.1.0.11/8
      nameservers:
        search: [labo.local]
        addresses: [192.168.1.9]
      routes:
      - to: default
        via: 10.0.0.1
    # Inter hyperVisor
    enp4s0f0:
      dhcp4: no
    enp4s0f1:
      dhcp4: no
    wlo1:
      dhcp4: no
    enp42s0:
      dhcp4: no   
```

設定を有効化する

```
# netplan apply
```

## リモートログインのための環境作成

### ホームディレクトリの sshの環境を設定する
筆者の環境では、プライベート・オブジェクトストレージから、ssh鍵などをダウンロードして、
ubuntuとrootのホームディレクトに展開する。

```
$ curl -O http://10.1.0.12:9000/utils/ssh-key.tar
$ tar xvf ssh-key.tar
$ rm ssh-key.tar
```

### rootディレクトリの sshの環境を設定する
```
$ sudo -s
# cd
# curl -O http://10.1.0.12:9000/utils/ssh-key.tar
# tar xvf ssh-key.tar
# chown -R root:root .ssh
# rm ssh-key.tar

```

## 他環境

- ネットワークは、[ネットワーク設定](network-setup.md)
- marmotのビルドには、[Go言語をインストール](/home/ubuntu/marmot/docs)







