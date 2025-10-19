# データベースの内容の説明と操作


## データベースのハイパーバイザー設定の初期化

事前にゴミデータが存在すれば、削除しておく
```
$ export ETCDCTL_API=3
$ etcdctl --endpoints=localhost:12379 get --prefix SEQNO
$ etcdctl --endpoints=localhost:12379 get --prefix hv
$ etcdctl --endpoints=localhost:12379 get --prefix vm
$ etcdctl --endpoints=localhost:12379 get --prefix OSI
```


データの確認
```
$ etcdctl --endpoints=localhost:12379 get --prefix SEQNO
SEQNO_LVDATA
{"Serial":100,"Start":100,"Step":1,"Key":"LVDATA"}
SEQNO_LVOS
{"Serial":100,"Start":100,"Step":1,"Key":"LVOS"}
SEQNO_VM
{"Serial":100,"Start":100,"Step":1,"Key":"VM"}

$ etcdctl --endpoints=localhost:12379 get --prefix hv
hvc
{"Nodename":"hvc","Cpu":8,"Memory":16384,"IpAddr":"172.16.0.20","FreeCpu":8,"FreeMemory":16384,"Key":"hvc","Status":2,"StgPool":[{"VolGroup":"vg1","FreeCap":0,"VgCap":0,"Type":"ssd"},{"VolGroup":"vg2","FreeCap":0,"VgCap":0,"Type":"nvme"},{"VolGroup":"vg3","FreeCap":0,"VgCap":0,"Type":"hdd"}]}

$ etcdctl --endpoints=localhost:12379 get --prefix vm

$ etcdctl --endpoints=localhost:12379 get --prefix OSI
OSI_ubuntu22.04
{"LogicaVol":"lv01","VolumeGroup":"vg1","OsVariant":"ubuntu22.04"}
```
