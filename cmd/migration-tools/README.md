# hv-admin 

ハイパーバイザーの資源を登録するコマンド

## etcdのインストールと設定

```
apt-get update -y && apt install etcd
systemctl edit etcd.service
```
 
以下３行を追加して、ドロップインを作成

```
[Service]
Environment=ETCD_LISTEN_CLIENT_URLS="http://0.0.0.0:2379"
Environment=ETCD_ADVERTISE_CLIENT_URLS="http://0.0.0.0:2379"
```


## ストレージの情報収集

```
# pvdisplay 
  --- Physical volume ---
  PV Name               /dev/sdb
  VG Name               vg1
  PV Size               931.51 GiB / not usable 1.71 MiB
  Allocatable           yes 
  PE Size               4.00 MiB
  Total PE              238467
  Free PE               197507
  Allocated PE          40960
  PV UUID               a6UH0h-261Y-ntzn-tuni-ruif-eP3q-iPGlgh
   
  --- Physical volume ---
  PV Name               /dev/sda
  VG Name               vg3
  PV Size               <2.73 TiB / not usable 4.46 MiB
  Allocatable           yes 
  PE Size               4.00 MiB
  Total PE              715396
  Free PE               100996
  Allocated PE          614400
  PV UUID               z746gx-Hr1D-Hc6w-QOd8-xVUk-LwFn-I3jbah
   
  --- Physical volume ---
  PV Name               /dev/nvme0n1
  VG Name               vg2
  PV Size               931.51 GiB / not usable 1.71 MiB
  Allocatable           yes 
  PE Size               4.00 MiB
  Total PE              238467
  Free PE               59267
  Allocated PE          179200
  PV UUID               4MRQJY-sO18-sSuy-OPUG-FZer-IoaO-t3Lpk5
```

## 論理ボリューム
使用中のLVのリストを表示

```
# lvs
  LV       VG  Attr       LSize    Pool Origin Data%  Meta%  Move Log Cpy%Sync Convert
  lv01     vg1 owi-a-s---   16.00g                                                    
  lv02     vg1 -wi-a-----   16.00g                                                    
  lv03     vg1 owi-a-s---   16.00g                                                    
  oslv0212 vg1 swi-a-s---   16.00g      lv01   18.96                                  
  oslv0289 vg1 swi-a-s---   16.00g      lv03   66.96                                  
  oslv0337 vg1 swi-a-s---   16.00g      lv01   44.62                                  
  oslv0338 vg1 swi-a-s---   16.00g      lv01   35.68                                  
  oslv0339 vg1 swi-a-s---   16.00g      lv01   33.06                                  
  oslv0340 vg1 swi-a-s---   16.00g      lv01   16.91                                  
  oslv0341 vg1 swi-a-s---   16.00g      lv01   15.72                                  
  data0314 vg2 -wi-a-----  100.00g                                                    
  data0366 vg2 -wi-a-----  120.00g                                                    
  data0367 vg2 -wi-a-----  120.00g                                                    
  data0368 vg2 -wi-a-----  120.00g                                                    
  data0369 vg2 -wi-a-----  120.00g                                                    
  data0370 vg2 -wi-a-----  120.00g                                                    
  data0215 vg3 -wi-a-----  100.00g                                                    
  data0216 vg3 -wi-a-----  100.00g                                                    
  data0217 vg3 -wi-a-----  100.00g                                                    
  data0218 vg3 -wi-a-----  100.00g                                                    
  data0226 vg3 -wi-a----- 1000.00g                                                    
  data0227 vg3 -wi-a----- 1000.00g    
```


## CPUのコア数の求め方

```
# lscpu
Architecture:             x86_64
  CPU op-mode(s):         32-bit, 64-bit
  Address sizes:          39 bits physical, 48 bits virtual
  Byte Order:             Little Endian
CPU(s):                   16
  On-line CPU(s) list:    0-15
Vendor ID:                GenuineIntel
  Model name:             Intel(R) Core(TM) i9-9900KF CPU @ 3.60GHz
    CPU family:           6
    Model:                158
    Thread(s) per core:   2
    Core(s) per socket:   8
    Socket(s):            1
    Stepping:             13
    CPU max MHz:          5000.0000
    CPU min MHz:          800.0000
    BogoMIPS:             7200.00
    Flags:                fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge m
                          ca cmov pat pse36 clflush dts acpi mmx fxsr sse sse2 s
                          s ht tm pbe syscall nx pdpe1gb rdtscp lm constant_tsc 
                          art arch_perfmon pebs bts rep_good nopl xtopology nons
                          top_tsc cpuid aperfmperf pni pclmulqdq dtes64 monitor 
                          ds_cpl vmx est tm2 ssse3 sdbg fma cx16 xtpr pdcm pcid 
                          sse4_1 sse4_2 x2apic movbe popcnt tsc_deadline_timer a
                          es xsave avx f16c rdrand lahf_lm abm 3dnowprefetch cpu
                          id_fault epb ssbd ibrs ibpb stibp ibrs_enhanced tpr_sh
                          adow flexpriority ept vpid ept_ad fsgsbase tsc_adjust 
                          bmi1 avx2 smep bmi2 erms invpcid mpx rdseed adx smap c
                          lflushopt intel_pt xsaveopt xsavec xgetbv1 xsaves dthe
                          rm ida arat pln pts hwp hwp_notify hwp_act_window hwp_
                          epp vnmi md_clear flush_l1d arch_capabilities
Virtualization features:  
  Virtualization:         VT-x
Caches (sum of all):      
  L1d:                    256 KiB (8 instances)
  L1i:                    256 KiB (8 instances)
  L2:                     2 MiB (8 instances)
  L3:                     16 MiB (1 instance)
NUMA:                     
  NUMA node(s):           1
  NUMA node0 CPU(s):      0-15
Vulnerabilities:          
  Gather data sampling:   Mitigation; Microcode
  Itlb multihit:          KVM: Mitigation: VMX disabled
  L1tf:                   Not affected
  Mds:                    Not affected
  Meltdown:               Not affected
  Mmio stale data:        Mitigation; Clear CPU buffers; SMT vulnerable
  Reg file data sampling: Not affected
  Retbleed:               Mitigation; Enhanced IBRS
  Spec rstack overflow:   Not affected
  Spec store bypass:      Mitigation; Speculative Store Bypass disabled via prct
                          l
  Spectre v1:             Mitigation; usercopy/swapgs barriers and __user pointe
                          r sanitization
  Spectre v2:             Mitigation; Enhanced / Automatic IBRS; IBPB conditiona
                          l; RSB filling; PBRSB-eIBRS SW sequence; BHI SW loop, 
                          KVM SW loop
  Srbds:                  Mitigation; Microcode
  Tsx async abort:        Mitigation; TSX disabled

```

## 設定ファイルを編集

```
ubuntu@hv2:~/marmot/cmd/hv-admin$ vi hypervisor-config-hv2.yaml
```

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

