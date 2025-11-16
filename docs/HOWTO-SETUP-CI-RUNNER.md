# GitHub Runnner の仮想サーバーのセットアップ方法


## ホスト名

```
# hostname
hvc
```


## ストレージ設定

CIで使用している 物理ボリューム
```
# pvs
  PV         VG  Fmt  Attr PSize    PFree   
  /dev/vdc   vg1 lvm2 a--  <100.00g  <84.00g
  /dev/vdd   vg2 lvm2 a--  <100.00g <100.00g
```

物理ボリュームの詳細
```
# pvdisplay
  --- Physical volume ---
  PV Name               /dev/vdd
  VG Name               vg2
  PV Size               100.00 GiB / not usable 4.00 MiB
  Allocatable           yes 
  PE Size               4.00 MiB
  Total PE              25599
  Free PE               25599
  Allocated PE          0
  PV UUID               T1sikG-uEUT-gjBX-zb5T-K4zH-55Or-72GRkO
   
  --- Physical volume ---
  PV Name               /dev/vdc
  VG Name               vg1
  PV Size               100.00 GiB / not usable 4.00 MiB
  Allocatable           yes 
  PE Size               4.00 MiB
  Total PE              25599
  Free PE               21503
  Allocated PE          4096
  PV UUID               PIJdjo-FVeu-TcBG-WTe1-JpV8-3TH3-be4vEK
```

仮想マシンのOSテンプレートのイメージ
```
# lvs
  LV   VG  Attr       LSize  Pool Origin Data%  Meta%  Move Log Cpy%Sync Convert
  lv01 vg1 -wi-a----- 16.00g        
```

## CPUの情報

```
# lscpu
Architecture:            x86_64
  CPU op-mode(s):        32-bit, 64-bit
  Address sizes:         40 bits physical, 48 bits virtual
  Byte Order:            Little Endian
CPU(s):                  4
  On-line CPU(s) list:   0-3
Vendor ID:               GenuineIntel
  Model name:            Intel Core Processor (Skylake, IBRS)
    CPU family:          6
    Model:               94
    Thread(s) per core:  1
    Core(s) per socket:  1
    Socket(s):           4
    Stepping:            3
    BogoMIPS:            7199.99
    Flags:               fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat pse36 clflush mmx fxsr sse sse2 ss syscall nx pdpe1gb rdtscp lm constant_tsc rep_good nopl xtopology cpuid tsc_known_freq pni pclmulqdq vmx ssse3 fma cx16 pcid sse4_1 sse4_2 x2ap
                         ic movbe popcnt tsc_deadline_timer aes xsave avx f16c rdrand hypervisor lahf_lm abm 3dnowprefetch cpuid_fault invpcid_single ssbd ibrs ibpb stibp ibrs_enhanced tpr_shadow vnmi flexpriority ept vpid ept_ad fsgsbase tsc_adjust bmi1 avx2 smep bmi2
                          erms invpcid rdseed adx smap clflushopt xsaveopt xsavec xgetbv1 xsaves arat umip md_clear arch_capabilities
Virtualization features: 
  Virtualization:        VT-x
  Hypervisor vendor:     KVM
  Virtualization type:   full
Caches (sum of all):     
  L1d:                   128 KiB (4 instances)
  L1i:                   128 KiB (4 instances)
  L2:                    16 MiB (4 instances)
  L3:                    64 MiB (4 instances)
NUMA:                    
  NUMA node(s):          1
  NUMA node0 CPU(s):     0-3
Vulnerabilities:         
  Gather data sampling:  Not affected
  Itlb multihit:         Not affected
  L1tf:                  Not affected
  Mds:                   Not affected
  Meltdown:              Not affected
  Mmio stale data:       Vulnerable: Clear CPU buffers attempted, no microcode; SMT Host state unknown
  Retbleed:              Mitigation; Enhanced IBRS
  Spec rstack overflow:  Not affected
  Spec store bypass:     Mitigation; Speculative Store Bypass disabled via prctl and seccomp
  Spectre v1:            Mitigation; usercopy/swapgs barriers and __user pointer sanitization
  Spectre v2:            Mitigation; Enhanced IBRS, IBPB conditional, RSB filling, PBRSB-eIBRS SW sequence
  Srbds:                 Unknown: Dependent on hypervisor status
  Tsx async abort:       Mitigation; TSX disabled
```

## メモリのサイズ

```
root@hvc:/home/ubuntu/marmot# free -h
               total        used        free      shared  buff/cache   available
Mem:            15Gi       1.6Gi        12Gi        21Mi       1.7Gi        13Gi
Swap:             0B          0B          0B
root@hvc:/home/ubuntu/marmot# free
               total        used        free      shared  buff/cache   available
Mem:        16373260     1675688    12883084       22372     1814488    14315352
Swap:              0           0           0
```

## 設定ファイルの編集

```
ubuntu@hvc:~/marmot/cmd/hv-admin$ vi hypervisor-config-hvc.yaml
ubuntu@hvc:~/marmot/cmd/hv-admin$ cat hypervisor-config-hvc.yaml 
hv_spec:
  - name: "hvc"
    cpu: 8
    free_cpu: 7
    ram: 16
    free_ram: 14
    ip_addr: "172.16.0.20"
    storage_pool:
      - vg: "vg1"
        type: "ssd"
      - vg: "vg2"
        type: "nvme"

image_template:
  - name: "ubuntu22.04"
    volumegroup: "vg1"
    logicalvolume: "lv01"

seqno:
  - name: "LVOS"
    start: 100
    step: 1
  - name: "LVDATA"
    start: 100
    step: 1
  - name: "VM"
    start: 100
    step: 1
```

## ハイパーバイザー設定の初期化

事前にゴミデータが存在すれば、削除しておく
```
$ export ETCDCTL_API=3
$ etcdctl --endpoints=localhost:12379 get --prefix SEQNO
$ etcdctl --endpoints=localhost:12379 get --prefix hv
$ etcdctl --endpoints=localhost:12379 get --prefix vm
$ etcdctl --endpoints=localhost:12379 get --prefix OSI
```

データの読み込み
```
$ hv-admin -config hypervisor-config-hvc.yaml 
{hvc 8 7 16 14 172.16.0.20 [{vg1 ssd} {vg2 nvme} ]}
```

セットしたデータの確認
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


## 起動の確認

```
$ sudo systemctl start marmot
$ systemctl status marmot
● marmot.service - marmot - vm cluster service
     Loaded: loaded (/lib/systemd/system/marmot.service; disabled; vendor preset: enabled)
     Active: active (running) since Sun 2025-08-24 06:27:03 UTC; 18ms ago
       Docs: https://github.com/takara9/marmot
             man:marmot
   Main PID: 16472 (sh)
      Tasks: 9 (limit: 19051)
     Memory: 8.0M
        CPU: 17ms
     CGroup: /system.slice/marmot.service
             ├─16472 /bin/sh -c "cd /usr/local/marmot;/usr/local/marmot/vm-server --node=hvc --etcd=http://localhost:12379"
             └─16473 /usr/local/marmot/vm-server --node=hvc --etcd=http://localhost:12379

Aug 24 06:27:03 hvc systemd[1]: Started marmot - vm cluster service.
Aug 24 06:27:03 hvc sh[16473]: node =  hvc
```
