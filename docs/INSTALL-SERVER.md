# ハイパーバイザーノードの構成


## パッケージ

marmot を動かすには、Ubuntu 20.04 に以下のパッケージをインストールする必要がある。

```
$ sudo apt-get update -y
$ sudo apt-get install git curl gcc make kpartx
$ sudo apt-get install virt-top  virt-manager libvirt-dev libvirt-clients libvirt-daemon qemu-kvm qemu openvswitch-switch openvswitch-common openvswitch-doc
```


## NICの設定

マザーボードのネットワークI/Fを管理用LANに接続して、固定のIPアドレスを設定する。


/etc/netplan/01-network-manager-all.yaml を編集する

```
# Let NetworkManager manage all devices on this system
#network:
#  version: 2
#  renderer: NetworkManager
network:
  version: 2
  renderer: networkd
  ethernets:
    eno1:
      addresses:
      - 10.1.0.12/8
      nameservers:
        search: [labo.local]
        addresses: [192.168.1.9]
      routes:
      - to: default
        via: 10.0.0.1
    enp4s0f0:
      dhcp4: no
    enp4s0f1:
      dhcp4: no
    wlp3s0:
      dhcp4: no
```

設定を有効化する

```
# netplan apply
```


## ホームディレクトリの sshの環境を設定する

```
$ cd
$ tar xvf ssh.tar
```

## rootディレクトリの sshの環境を設定する

```
root@hv2:~# pwd
/root
root@hv2:~# mkdir .ssh
root@hv2:~# cd .ssh
root@hv2:~/.ssh# vi authorized_keys
root@hv2:~/.ssh# chmod 0600 authorized_keys
```

## 他環境

- ネットワークは、[ネットワーク設定](network-setup.md)
- marmotのビルドには、[Go言語をインストール](/home/ubuntu/marmot/docs)







