# ネットワークの設定

Nested Virtualizationのネットワーク環境設定について記述する。

仮想サーバー上でハイパーバイザーを動かすのは、CI用の自動テスト環境を実行するためである。

## ベアメタルのハイパーバイザー側

こちらは通常のハイパーバイザーの設定と同じである。

### Open vSwitchの設定

物理ポートへトランクをセットする

~~~
# ovs-vsctl add-br ovsbr0
# ovs-vsctl add-port ovsbr0 enp5s0f0
# ovs-vsctl set port enp5s0f0 trunk=1001,1002
~~~

確認コマンドの実行
L2スイッチ側のVLAN設定も必要なので、事前にL2スイッチの設定を済ませておくこと。

~~~
root@hv1:/home/ubuntu# ovs-vsctl show
f55a08ab-9ca3-495a-bbe2-7388e39e6df0
    Bridge ovsbr0
        Port enp6s0f0
            trunks: [1001, 1002]
            Interface enp6s0f0
        Port ovsbr0
            Interface ovsbr0
                type: internal
    ovs_version: "2.13.8"
~~~

### Libvirtの設定

設定ファイルは、VLANタグ 1001 は、プライベートネットワーク、VLANタグ 1002 は、パブリックネットワークとして対応づける。
通常の仮想サーバーは、「vlan-1001」と「vlan-1002」にNICを繋げれば良い。
一方、Nested VMのCI用サーバーは、「vlan-all」に対してNICを繋ぐことで、孫VMをVLANへブリッジする。

~~~
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
~~~

virsh の仮想ネットワークに追加して、アクティブにする。そして、自動起動にする。

~~~
# virsh net-define ovs-network.xml
# virsh net-start ovs-network
# virsh net-autostart ovs-network
# virsh net-list
 Name          State    Autostart   Persistent
------------------------------------------------
 default       active   yes         yes
 ovs-network   active   yes         yes
~~~


## 仮想サーバーのハイパーバイザー側


### 仮想ハイパーバイザーのNIC設定

Netsted VM で仮想HVによるテスト環境を作る仮想サーバーには、以下の設定を追加してVLANトランクのインタフェースを接続する。

~~~
root@hv1:/home/ubuntu# virsh dumpxml vm_server1_0104
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


### 仮想ハイパーバイザーの Open vSwitch の設定

前述で追加したNICをUPするように、netplanの設定を追加して、`netplan apply` を実行する

~~~
root@hv0:/etc/netplan# cat 00-nic.yaml 
network:
  version: 2
  ethernets:
    enp6s0:
      addresses:
        - 172.16.99.101/16
      nameservers:
        search: [labo.local]
        addresses: [172.16.0.4]
    enp7s0:
      addresses:
        - 192.168.1.201/24
      routes:
        - to: default
          via: 192.168.1.1
    enp8s0:
      dhcp4: no
~~~

確認として、以下のコマンドを実行する。 「enp8s0」 が UP になっていることを確認する。

~~~
root@hv0:/home/ubuntu# ip l
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
2: enp6s0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP mode DEFAULT group default qlen 1000
    link/ether 02:c3:ce:79:98:85 brd ff:ff:ff:ff:ff:ff
3: enp7s0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP mode DEFAULT group default qlen 1000
    link/ether 02:89:3c:96:6c:da brd ff:ff:ff:ff:ff:ff
4: enp8s0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel master ovs-system state UP mode DEFAULT group default qlen 1000
    link/ether 52:54:00:23:d6:db brd ff:ff:ff:ff:ff:ff
＜以下省略＞
~~~

Open vSwitchの設定を入れる。「ovsbr0」に、先に設定したトランクポート「enp8s0」を繋いぐ。

~~~
# ovs-vsctl add-br ovsbr0
# ovs-vsctl add-port ovsbr0 enp8s0
# ovs-vsctl set port enp8s0 trunk=1001,1002
~~~


### Libvirtのネットワークは、一般同様で良い。

Nested VMのテスト環境と、通常環境は、同様の設定で良い。

~~~
root@hv0:/etc/netplan# virsh net-list
 Name          State    Autostart   Persistent
------------------------------------------------
 default       active   yes         yes
 ovs-network   active   yes         yes

root@hv0:/etc/netplan# virsh net-dumpxml ovs-network
<network connections='4'>
  <name>ovs-network</name>
  <uuid>70eec67f-ac20-4d44-9283-56e6531bf50e</uuid>
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
~~~

