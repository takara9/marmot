ubuntu@hv2:~$ sudo -s
root@hv2:/home/ubuntu# cd /etc/netplan
root@hv2:/etc/netplan# ls -la
total 20
drwxr-xr-x   2 root root  4096  4月 12 13:41 .
drwxr-xr-x 142 root root 12288  4月 12 13:43 ..
-rw-r--r--   1 root root   499  4月 12 13:41 01-network-manager-all.yaml
root@hv2:/etc/netplan# cat 01-network-manager-all.yaml 
# Let NetworkManager manage all devices on this system
#network:
#  version: 2
#  renderer: NetworkManager
network:
  version: 2
  renderer: networkd
  ethernets:
    eno1:
      addresses:
      - 10.1.0.12/8
      nameservers:
        search: [labo.local]
        addresses: [192.168.1.9]
      routes:
      - to: default
        via: 10.0.0.1
      dhcp6: no
    enp4s0f0:
      dhcp4: no
      dhcp6: no
    enp4s0f1:
      dhcp4: no
      dhcp6: no
    wlp3s0:
      dhcp4: no
      dhcp6: no
root@hv2:/etc/netplan# lsblk
NAME                MAJ:MIN RM   SIZE RO TYPE MOUNTPOINTS
loop0                 7:0    0     4K  1 loop /snap/bare/5
loop1                 7:1    0  63.8M  1 loop /snap/core20/2717
loop2                 7:2    0  63.8M  1 loop /snap/core20/2769
loop3                 7:3    0    74M  1 loop /snap/core22/2292
loop4                 7:4    0    74M  1 loop /snap/core22/2411
loop5                 7:5    0  66.8M  1 loop /snap/core24/1587
loop6                 7:6    0 254.9M  1 loop /snap/firefox/7967
loop7                 7:7    0 273.7M  1 loop /snap/firefox/8107
loop8                 7:8    0 349.7M  1 loop /snap/gnome-3-38-2004/143
loop9                 7:9    0 516.2M  1 loop /snap/gnome-42-2204/226
loop10                7:10   0 531.4M  1 loop /snap/gnome-42-2204/247
loop11                7:11   0 606.1M  1 loop /snap/gnome-46-2404/153
loop12                7:12   0  91.7M  1 loop /snap/gtk-common-themes/1535
loop13                7:13   0   395M  1 loop /snap/mesa-2404/1165
loop14                7:14   0  12.3M  1 loop /snap/snap-store/959
loop15                7:15   0  12.2M  1 loop /snap/snap-store/1216
loop16                7:16   0  48.1M  1 loop /snap/snapd/25935
loop17                7:17   0  48.4M  1 loop /snap/snapd/26382
loop18                7:18   0   576K  1 loop /snap/snapd-desktop-integration/315
loop19                7:19   0   576K  1 loop /snap/snapd-desktop-integration/343
sda                   8:0    1  28.9G  0 disk 
├─sda1                8:1    1   5.9G  0 part 
├─sda2                8:2    1     5M  0 part 
├─sda3                8:3    1   300K  0 part 
└─sda4                8:4    1    23G  0 part 
sdb                   8:16   0   2.7T  0 disk 
└─sdb1                8:17   0   2.7T  0 part 
  └─vg3-minio--data 252:10   0     1T  0 lvm  /minio-data
sdc                   8:32   0 931.5G  0 disk 
├─vg1-lv01          252:6    0    16G  0 lvm  
├─vg1-lv02          252:7    0    16G  0 lvm  
├─vg1-lv03          252:8    0    16G  0 lvm  
├─vg1-lv10          252:9    0    16G  0 lvm  
├─vg1-lv04-real     252:11   0    25G  0 lvm  
│ ├─vg1-lv04        252:12   0    25G  0 lvm  
│ ├─vg1-oslv0589    252:14   0    25G  0 lvm  
│ ├─vg1-oslv0590    252:16   0    25G  0 lvm  
│ └─vg1-oslv0591    252:18   0    25G  0 lvm  
├─vg1-oslv0589-cow  252:13   0    16G  0 lvm  
│ └─vg1-oslv0589    252:14   0    25G  0 lvm  
├─vg1-oslv0590-cow  252:15   0    16G  0 lvm  
│ └─vg1-oslv0590    252:16   0    25G  0 lvm  
├─vg1-oslv0591-cow  252:17   0    16G  0 lvm  
│ └─vg1-oslv0591    252:18   0    25G  0 lvm  
├─vg1-data0704      252:19   0   100G  0 lvm  
├─vg1-data0705      252:20   0   100G  0 lvm  
├─vg1-data0707      252:21   0   100G  0 lvm  
└─vg1-data0708      252:22   0   100G  0 lvm  
sdd                   8:48   0 476.9G  0 disk 
├─sdd1                8:49   0     1M  0 part 
├─sdd2                8:50   0   513M  0 part /boot/efi
└─sdd3                8:51   0 476.4G  0 part /var/snap/firefox/common/host-hunspell
                                              /
nvme0n1             259:0    0 931.5G  0 disk 
├─vg2-data1000      252:0    0    20G  0 lvm  
├─vg2-data0701      252:1    0   100G  0 lvm  
├─vg2-data0702      252:2    0   100G  0 lvm  
├─vg2-data0703      252:3    0   100G  0 lvm  
├─vg2-data0706      252:4    0   100G  0 lvm  
└─vg2-data0709      252:5    0   100G  0 lvm  
root@hv2:/etc/netplan# vgdisply
Command 'vgdisply' not found, did you mean:
  command 'vgdisplay' from deb lvm2 (2.03.11-2.1ubuntu5)
Try: apt install <deb name>
root@hv2:/etc/netplan# vgdisplay
  --- Volume group ---
  VG Name               vg1
  System ID             
  Format                lvm2
  Metadata Areas        1
  Metadata Sequence No  7541
  VG Access             read/write
  VG Status             resizable
  MAX LV                0
  Cur LV                12
  Open LV               7
  Max PV                0
  Cur PV                1
  Act PV                1
  VG Size               931.51 GiB
  PE Size               4.00 MiB
  Total PE              238467
  Alloc PE / Size       137472 / 537.00 GiB
  Free  PE / Size       100995 / 394.51 GiB
  VG UUID               uEeru6-iKGJ-4rko-PfOt-SBzn-D0dp-WGX4EB
   
  --- Volume group ---
  VG Name               vg3
  System ID             
  Format                lvm2
  Metadata Areas        1
  Metadata Sequence No  66
  VG Access             read/write
  VG Status             resizable
  MAX LV                0
  Cur LV                1
  Open LV               1
  Max PV                0
  Cur PV                1
  Act PV                1
  VG Size               <2.73 TiB
  PE Size               4.00 MiB
  Total PE              715396
  Alloc PE / Size       262144 / 1.00 TiB
  Free  PE / Size       453252 / <1.73 TiB
  VG UUID               vphnkf-SB1k-tYAQ-cJxN-91z4-GqYr-iTzquZ
   
  --- Volume group ---
  VG Name               vg2
  System ID             
  Format                lvm2
  Metadata Areas        1
  Metadata Sequence No  943
  VG Access             read/write
  VG Status             resizable
  MAX LV                0
  Cur LV                6
  Open LV               5
  Max PV                0
  Cur PV                1
  Act PV                1
  VG Size               931.51 GiB
  PE Size               4.00 MiB
  Total PE              238467
  Alloc PE / Size       133120 / 520.00 GiB
  Free  PE / Size       105347 / 411.51 GiB
  VG UUID               iGCbKh-Lbw6-3TfY-Gwgd-LTnD-ouWf-BRxnld
   
root@hv2:/etc/netplan# 

root@hv2:/etc/netplan# df -h
Filesystem                   Size  Used Avail Use% Mounted on
tmpfs                         13G  2.6M   13G   1% /run
/dev/sdd3                    468G  100G  345G  23% /
tmpfs                         63G     0   63G   0% /dev/shm
tmpfs                        5.0M  4.0K  5.0M   1% /run/lock
efivarfs                     192K   50K  138K  27% /sys/firmware/efi/efivars
tmpfs                         63G     0   63G   0% /run/qemu
/dev/mapper/vg3-minio--data 1007G   88G  868G  10% /minio-data
/dev/sdd2                    512M  6.1M  506M   2% /boot/efi
tmpfs                         13G   76K   13G   1% /run/user/128
tmpfs                         13G   64K   13G   1% /run/user/1000

root@hv2:/etc/netplan# lvs
  LV         VG  Attr       LSize   Pool Origin Data%  Meta%  Move Log Cpy%Sync Convert
  data0704   vg1 -wi-ao---- 100.00g                                                    
  data0705   vg1 -wi-ao---- 100.00g                                                    
  data0707   vg1 -wi-ao---- 100.00g                                                    
  data0708   vg1 -wi-ao---- 100.00g                                                    
  lv01       vg1 -wi-a-----  16.00g                                                    
  lv02       vg1 -wi-a-----  16.00g                                                    
  lv03       vg1 -wi-a-----  16.00g                                                    
  lv04       vg1 owi-a-s---  25.00g                                                    
  lv10       vg1 -wi-a-----  16.00g                                                    
  oslv0589   vg1 swi-aos---  16.00g      lv04   66.90                                  
  oslv0590   vg1 swi-aos---  16.00g      lv04   52.68                                  
  oslv0591   vg1 swi-aos---  16.00g      lv04   93.39                                  
  data0701   vg2 -wi-ao---- 100.00g                                                    
  data0702   vg2 -wi-ao---- 100.00g                                                    
  data0703   vg2 -wi-ao---- 100.00g                                                    
  data0706   vg2 -wi-ao---- 100.00g                                                    
  data0709   vg2 -wi-ao---- 100.00g                                                    
  data1000   vg2 -wi-a-----  20.00g                                                    
  minio-data vg3 -wi-ao----   1.00t      
  