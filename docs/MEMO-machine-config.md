# マシンの構成


## hv3

### CPU
Architecture:                       x86_64
CPU op-mode(s):                     32-bit, 64-bit
Byte Order:                         Little Endian
Address sizes:                      43 bits physical, 48 bits virtual
CPU(s):                             32
Thread(s) per core:                 2
Core(s) per socket:                 16
NUMA node(s):                       1
Vendor ID:                          AuthenticAMD
CPU family:                         23
Model:                              113
Model name:                         AMD Ryzen 9 3950X 16-Core Processor


### Storage
Disk /dev/nvme0n1: 931.53 GiB, Disk model: CSSD-M2B1TPG3VNF  
Disk /dev/sda: 931.53 GiB, Disk model: SanDisk SDSSDH3
Disk /dev/sdb: 931.53 GiB, Disk model: WDC WD10EADS-00L
Disk /dev/sdc: 931.53 GiB, Disk model: SanDisk SDSSDH31

sda                  8:0    0 931.5G  0 disk 
├─sda1               8:1    0   1.1G  0 part /boot/efi
└─sda2               8:2    0 930.5G  0 part /
sdb                  8:16   0 931.5G  0 disk 
├─vg3-data0542     253:3    0   100G  0 lvm  
├─vg3-data0543     253:4    0   100G  0 lvm  
└─vg3-data0557     253:5    0    10G  0 lvm  
sdc                  8:32   0 931.5G  0 disk 
├─vg1-lv01-real    253:6    0    16G  0 lvm  
│ ├─vg1-lv01       253:7    0    16G  0 lvm  
│ ├─vg1-oslv0530   253:9    0    16G  0 lvm  

2: enp4s0f0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq master ovs-system state UP mode DEFAULT group default qlen 1000
    link/ether 80:61:5f:0d:1c:24 brd ff:ff:ff:ff:ff:ff
3: enp4s0f1: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq state UP mode DEFAULT group default qlen 1000
    link/ether 80:61:5f:0d:1c:25 brd ff:ff:ff:ff:ff:ff
4: enp5s0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq state UP mode DEFAULT group default qlen 1000
    link/ether 24:4b:fe:04:fb:e6 brd ff:ff:ff:ff:ff:ff

### NIC
root@hv3:/etc/netplan# cat 00-installer-config.yaml 
network:
  ethernets:
    enp4s0f0:
      dhcp4: no
    enp4s0f1:
      dhcp4: no
    enp5s0:
      addresses:
      - 10.1.0.13/8
      gateway4: 10.0.0.1
      nameservers:
        addresses:
        - 192.168.1.9
        search:
        - labo.local
  version: 2


## hv4

### CPU
Architecture:                    x86_64
CPU op-mode(s):                  32-bit, 64-bit
Byte Order:                      Little Endian
Address sizes:                   43 bits physical, 48 bits virtual
CPU(s):                          32
Thread(s) per core:              2
Core(s) per socket:              16
Model name:                      AMD Ryzen 9 3950X 16-Core Processor


### Storage
Disk /dev/nvme0n1: 931.53 GiB, Disk model: CSSD-M2B1TPG3VNF
Disk /dev/sda: 931.53 GiB, Disk model: SanDisk SDSSDH3 
Disk /dev/sdb: 931.53 GiB, Disk model: WDC WD10EZRX-00A
Disk /dev/sdc: 931.53 GiB, Disk model: SanDisk SDSSDH3 

sda                  8:0    0 931.5G  0 disk 
├─sda1               8:1    0   1.1G  0 part /boot/efi
└─sda2               8:2    0 930.5G  0 part /
sdb                  8:16   0 931.5G  0 disk 
├─vg3-data0539     253:6    0   100G  0 lvm  
├─vg3-data1047     253:7    0   100G  0 lvm  
sdc                  8:32   0 931.5G  0 disk 
├─vg1-lv01-real    253:11   0    16G  0 lvm  
│ ├─vg1-lv01       253:12   0    16G  0 lvm  
│ ├─vg1-oslv0533   253:14   0    16G  0 lvm 
nvme0n1            259:0    0 931.5G  0 disk 
├─vg2-data0556     253:0    0    40G  0 lvm  
├─vg2-data0799     253:1    0   120G  0 lvm  

2: enp4s0f0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq master ovs-system state UP mode DEFAULT group default qlen 1000
    link/ether 80:61:5f:0d:17:52 brd ff:ff:ff:ff:ff:ff
3: enp4s0f1: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq state UP mode DEFAULT group default qlen 1000
    link/ether 80:61:5f:0d:17:53 brd ff:ff:ff:ff:ff:ff
4: enp5s0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq state UP mode DEFAULT group default qlen 1000
    link/ether f0:2f:74:50:f6:9c brd ff:ff:ff:ff:ff:ff

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
