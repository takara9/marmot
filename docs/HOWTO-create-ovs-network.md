# インストール直後の状態から、ovs-network の作成手順

ovs-network は、２つ以上のVLANが含まれた VLAN Trunk です。


## インターフェースのリストを表示

活性化していない物理ポートのインターフェース名の候補をリストします。

```console
root@hv3:/etc/netplan# ip link show | grep -E '^[0-9]+: (enp|eth|wl)'
2: enp4s0f0: <BROADCAST,MULTICAST> mtu 1500 qdisc noop state DOWN mode DEFAULT group default qlen 1000
3: enp4s0f1: <BROADCAST,MULTICAST> mtu 1500 qdisc noop state DOWN mode DEFAULT group default qlen 1000
4: enp5s0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq state UP mode DEFAULT group default qlen 1000
```

## netplan で活性化して、IPアドレスの割当を抑止

/etc/netplan/50-cloud-init.yaml を編集して、VLAN Trunkを流すインターフェスを活性化する。

```yaml
network:
  version: 2
  ethernets:
    enp5s0:
      addresses:
      - "10.1.0.13/16"
      nameservers:
        addresses:
        - 192.168.1.10
        search:
        - labo.local
      routes:
      - to: "default"
        via: "10.1.0.1"
    # 以下の行を追加
    enp4s0f0:
      dhcp4: no
      dhcp6: no
    enp4s0f1:
      dhcp4: no
      dhcp6: no
```

編集後に `netplan apply` を実行して、インターフェースを起動する。

```console
root@hv3:/etc/netplan# vi 50-cloud-init.yaml 
root@hv3:/etc/netplan# netplan apply
```


## OpenvSwitch の設定

ブリッジポート `ovsbr0` を作成して、物理ポートと結び付け、VLANトランクを設定する。

```
# ovs-vsctl add-br ovsbr0
# ovs-vsctl set bridge ovsbr0 stp_enable=true
# ovs-vsctl add-port ovsbr0 enp4s0f0
# ovs-vsctl add-port ovsbr0 enp4s0f1
# ovs-vsctl set port enp4s0f0 trunk=1001,1002
```

確認コマンドの実行

```console
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
```

STPが有効化されていることを確認

```console
root@hv3:/etc/netplan# ovs-vsctl list bridge ovsbr0 | grep stp
rstp_enable         : false
rstp_status         : {}
status              : {stp_bridge_id="8000.80615f0d1c24", stp_designated_root="8000.80615f0d1c24", stp_root_path_cost="0"}
stp_enable          : true
```

## libvirt レベルの VLAN Trunk 対応のブリッジ作成

virsh で　仮想ネットワークを定義する

```xml
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
```

`virsh net-xxx` のコマンドで、有効化する

```console
root@hv3:/home/ubuntu# virsh net-define ovs-network.xml 
root@hv3:/home/ubuntu# virsh net-start ovs-network
root@hv3:/home/ubuntu# virsh net-autostart ovs-network
root@hv3:/home/ubuntu# virsh net-list
 Name          State    Autostart   Persistent
------------------------------------------------
 default       active   yes         yes
 host-bridge   active   yes         yes
 ovs-network   active   yes         yes
```

## marmot での認識確認

自動的に marmot に取り込まれる。

```cosnole
root@hv3:/home/ubuntu# mactl get net
NAME            NODE       BRIDGE        STATUS        AGE       IP-NET        
----            ---------  -----------   ----------    ---       --------------
default         hv3        virbr0        ACTIVE        10h       -             
host-bridge     hv3        br0           ACTIVE        1h        -             
ovs-network     hv3        ovsbr0        ACTIVE        1m        -     
```

