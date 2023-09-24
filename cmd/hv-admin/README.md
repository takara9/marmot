# hv-admin 

ハイパーバイザーの資源を登録するコマンド


## 使い方

登録する方法

```
$ hv-admin  -config hypervisor-config-hv1.yaml 
{hv1 12 12 64 64 10.1.0.11 [{vg1 ssd} {vg2 nvme} {vg3 hdd}]}
```

確認する方法

```
tkr@hmc:~/marmot/cmd/hv-admin$ mactl global-status

               *** SYSTEM STATUS ***
HV-NAME    ONL IPaddr          VCPU      RAM(MB)        Storage(GB) 
hv1        RUN 10.1.0.11         12/12    65536/65536   vg1(ssd):   899/931   vg2(nvme):  1907/1907  vg3(hdd):   931/931   

CLUSTER    VM-NAME          H-Visr STAT  VKEY                 VCPU  RAM    PubIP           PriIP           DATA STORAGE       
```


不要なデータを消す方法

```
$ export ETCDCTL_API=3
$ etcdctl get hv1
hv1
{"Nodename":"hv1","Cpu":12,"Memory":65536,"IpAddr":"10.1.0.11","FreeCpu":12,"FreeMemory":65536,"Key":"hv1","Status":2,"StgPool":[{"VolGroup":"vg1","FreeCap":899,"VgCap":931,"Type":"ssd"},{"VolGroup":"vg2","FreeCap":1907,"VgCap":1907,"Type":"nvme"},{"VolGroup":"vg3","FreeCap":931,"VgCap":931,"Type":"hdd"}]}

$ etcdctl del hv1
1
```

