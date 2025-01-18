# ネットワークの設定

事前にL2スイッチの設定を済ませておくこと。

## `Open vSwitch` の設定

ブリッジポート `ovsbr0` を作成して、物理ポートと結び付け、VLANトランクを設定する。

~~~
# ovs-vsctl add-br ovsbr0
# ovs-vsctl add-port ovsbr0 enp4s0f0
# ovs-vsctl add-port ovsbr0 enp4s0f1  　　⭐️⭐️ 後で確認 設定でループが発生した。
# ovs-vsctl set port enp4s0f0 trunk=1001,1002
~~~

確認コマンドの実行

~~~
# ovs-vsctl show
f55a08ab-9ca3-495a-bbe2-7388e39e6df0
    Bridge ovsbr0
        Port enp4s0f0
            trunks: [1001, 1002]
            Interface enp4s0f0
        Port ovsbr0
            Interface ovsbr0
                type: internal
    ovs_version: "2.13.8"
~~~


## `libvirt` の設定

ブリッジI/F `ovsbr0` と libvirt の仮想ネットワークを対応づけるため、
このディレクトリに存在する ovs-network.xml を適用する。

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

設定の確認

~~~
# virsh net-dumpxml ovs-network
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

以上で、marmotのための、ネットワーク設定は完了です。
