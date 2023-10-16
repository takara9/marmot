# ハイパーバイザーノードの構成






marmot を動かすには、Ubuntu 20.04 に以下のパッケージをインストールする必要がある。


```
$ sudo apt-get install git curl gcc make kpartx
$ sudo apt-get install virt-top  virt-manager libvirt-dev libvirt-clients libvirt-daemon qemu-kvm qemu openvswitch-switch openvswitch-common openvswitch-doc
```



## Open-vSwitch の設定

enp5s0f0をトランクポートへ設定する。

~
# ovs-vsctl add-br ovsbr0
# ovs-vsctl add-port ovsbr0 enp5s0f0
# ovs-vsctl set port enp5s0f0 trunk=1001,1002
~

設定内容確認

~
# ovs-vsctl show
1f641b32-4d5f-48e8-a25a-c1ac26fd8d3a
    Bridge ovsbr0
        Port enp5s0f0
            trunks: [1001, 1002]
            Interface enp5s0f0
        Port ovsbr0
            Interface ovsbr0
                type: internal
    ovs_version: "2.13.8"
~



## libvirt の仮想ネットワークの設定

Libvertに、OVSネットワークを設定する。

~
# cat ovs-network.xml
<network>
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
~

設定実行と確認

~
# virsh net-define ovs-network.xml
# virsh net-start ovs-network
# virsh net-autostart ovs-network
# virsh net-list
 Name          State    Autostart   Persistent
------------------------------------------------
 default       active   yes         yes
 ovs-network   active   yes         yes
~


## 仮想サーバーの設定


ネットワークインターフェスの追加

~
    <interface type='network'>
      <source network='default'/>
      <model type='virtio'/>
      <address type='pci' domain='0x0000' bus='0x01' slot='0x00' function='0x0'/>
    </interface>
    <interface type='network'>
      <source network='ovs-network' portgroup='vlan-1001'/>
      <model type='virtio'/>
      <address type='pci' domain='0x0000' bus='0x06' slot='0x00' function='0x0'/>
    </interface>
    <interface type='network'>
      <source network='ovs-network' portgroup='vlan-1002'/>
      <model type='virtio'/>
      <address type='pci' domain='0x0000' bus='0x07' slot='0x00' function='0x0'/>
    </interface>
~





marmotをビルドするには、Go言語をインストールする。




