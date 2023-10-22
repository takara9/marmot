# ハイパーバイザーノードの構成


## パッケージ

marmot を動かすには、Ubuntu 20.04 に以下のパッケージをインストールする必要がある。

```
$ sudo apt-get install git curl gcc make kpartx
$ sudo apt-get install virt-top  virt-manager libvirt-dev libvirt-clients libvirt-daemon qemu-kvm qemu openvswitch-switch openvswitch-common openvswitch-doc
```

## 他環境

- ネットワークは、[ネットワーク設定](network-setup.md)
- marmotのビルドには、[Go言語をインストール](/home/ubuntu/marmot/docs)







