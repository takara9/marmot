## ホストマシンのネットワーク設定
ゲストマシンで共有するインターフェースは、"dhcp4: no"でOFFにしておく。

```
ubuntu@hv4:~$ cat /etc/netplan/00-installer-config.yaml 
# This is the network config written by 'subiquity'
network:
  ethernets:
    enp4s0f0:
      dhcp4: no
    enp4s0f1:
      dhcp4: no
    enp5s0:
      addresses:
      - 10.1.0.14/8
      gateway4: 10.0.0.1
      nameservers:
        addresses:
        - 192.168.1.9
        search:
        - labo.local
  version: 2
```


## OpenvSwitch の設定

ブリッジポート `ovsbr0` を作成して、物理ポートと結び付け、VLANトランクを設定する。

```
# ovs-vsctl add-br ovsbr0
# ovs-vsctl add-port ovsbr0 enp4s0f0
# ovs-vsctl add-port ovsbr0 enp4s0f1  　　⭐️⭐️ 後で確認 設定でループが発生した。
# ovs-vsctl set port enp4s0f0 trunk=1001,1002
```

確認コマンドの実行

```
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




## 仮想ネットワークの定義

```
ubuntu@hv4:~$ sudo virsh net-dumpxml ovs-network 
<network connections='15'>
  <name>ovs-network</name>
  <uuid>a60aa8a2-1624-402b-ad82-39cae3512bff</uuid>
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


## 仮想マシンの定義

```
ubuntu@hv4:~$ sudo virsh dumpxml vm_node1_0786
前略
    </interface>
    <interface type='bridge'>
      <mac address='02:56:51:09:7a:e8'/>
      <source network='ovs-network' portgroup='vlan-1002' />
      <vlan>
        <tag id='1002'/>
      </vlan>
      <target dev='vnet3'/>
      <model type='virtio'/>
      <alias name='net1'/>
      <address type='pci' domain='0x0000' bus='0x07' slot='0x00' function='0x0'/>
    </interface>
```

## 仮想マシン上のOSの定義

```
ubuntu@node1:/etc/netplan$ cat 00-nic.yaml 
network:
  version: 2
  ethernets:
    enp6s0:
      addresses:
        - 172.16.0.31/16
      nameservers:
        search: [labo.local]
        addresses: [172.16.0.9]
    enp7s0:
      addresses:
        - 192.168.1.231/24
      routes:
        - to: default
          via: 192.168.1.1
```