# ハイパーバイザーノードの構成






marmot を動かすには、Ubuntu 20.04 に以下のパッケージをインストールする必要がある。


```
$ sudo apt-get git curl gcc kmartx
$ sudo apt-get install virt-top  virt-manager libvirt-dev libvirt-clients libvirt-daemon qemu-kvm qemu openvswitch-switch openvswitch-common openvswitch-doc
```


marmotをビルドするには、Go言語をインストールする。




