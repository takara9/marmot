# hv-admin 

ハイパーバイザーの資源を登録するコマンド


## 使い方

登録する方法

```
$ hv-admin  -config hypervisor-config-hv0.yaml 
{hv1 12 12 64 64 10.1.0.11 [{vg1 ssd} {vg2 nvme} {vg3 hdd}]}
```

確認する方法

```
tkr@hmc:~/marmot/cmd/hv-admin$ mactl global-status

               *** SYSTEM STATUS ***
HV-NAME    ONL IPaddr          VCPU      RAM(MB)        Storage(GB) 
hv0        RUN 10.1.0.11         12/12    65536/65536   vg1(ssd):   899/931   vg2(nvme):  1907/1907  vg3(hdd):   931/931   

CLUSTER    VM-NAME          H-Visr STAT  VKEY                 VCPU  RAM    PubIP           PriIP           DATA STORAGE       
```


不要なデータを消す方法

```
$ export ETCDCTL_API=3
$ etcdctl --endpoints=localhost:12379 get hv0
$ etcdctl del hv0
```

## ビルド方法

```console
make clean
make deps
make
make install
```

