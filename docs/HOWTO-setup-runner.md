# github action runner のセットアップ

## systemd-resolved.serviceを止める

```
root@runner1:/etc# systemctl status systemd-resolved.service 
● systemd-resolved.service - Network Name Resolution
     Loaded: loaded (/lib/systemd/system/systemd-resolved.service; enabled; vendor preset: enabled)
     Active: active (running) since Sat 2025-11-29 21:36:54 UTC; 33min ago
       Docs: man:systemd-resolved.service(8)
             man:org.freedesktop.resolve1(5)
             https://www.freedesktop.org/wiki/Software/systemd/writing-network-configuration-managers
             https://www.freedesktop.org/wiki/Software/systemd/writing-resolver-clients
   Main PID: 649 (systemd-resolve)
     Status: "Processing requests..."
      Tasks: 1 (limit: 19051)
     Memory: 9.3M
        CPU: 603ms
     CGroup: /system.slice/systemd-resolved.service
             └─649 /lib/systemd/systemd-resolved

Nov 29 22:09:47 runner1 systemd-resolved[649]: Using degraded feature set TCP instead of UDP for DNS server 172.16.0.4.
Nov 29 22:09:56 runner1 systemd-resolved[649]: Using degraded feature set UDP instead of TCP for DNS server 172.16.0.4.
Nov 29 22:09:59 runner1 systemd-resolved[649]: Using degraded feature set TCP instead of UDP for DNS server 172.16.0.4.
Nov 29 22:10:08 runner1 systemd-resolved[649]: Using degraded feature set UDP instead of TCP for DNS server 172.16.0.4.
Nov 29 22:10:11 runner1 systemd-resolved[649]: Using degraded feature set TCP instead of UDP for DNS server 172.16.0.4.
Nov 29 22:10:21 runner1 systemd-resolved[649]: Using degraded feature set UDP instead of TCP for DNS server 172.16.0.4.
Nov 29 22:10:24 runner1 systemd-resolved[649]: Using degraded feature set TCP instead of UDP for DNS server 172.16.0.4.
Nov 29 22:10:33 runner1 systemd-resolved[649]: Using degraded feature set UDP instead of TCP for DNS server 172.16.0.4.
Nov 29 22:10:36 runner1 systemd-resolved[649]: Using degraded feature set TCP instead of UDP for DNS server 172.16.0.4.
Nov 29 22:10:45 runner1 systemd-resolved[649]: Using degraded feature set UDP instead of TCP for DNS server 172.16.0.4.
root@runner1:/etc# systemctl stop systemd-resolved.service 
root@runner1:/etc# systemctl disable systemd-resolved.service 
Removed /etc/systemd/system/multi-user.target.wants/systemd-resolved.service.
Removed /etc/systemd/system/dbus-org.freedesktop.resolve1.service.
```

## /etc/resolve.confを編集する

```
root@runner1:/etc# vi /etc/resolv.conf 
root@runner1:/etc# tail resolv.conf 
# Third party programs should typically not access this file directly, but only
# through the symlink at /etc/resolv.conf. To manage man:resolv.conf(5) in a
# different way, replace this symlink by a static file or a different symlink.
#
# See man:systemd-resolved.service(8) for details about the supported modes of
# operation for /etc/resolv.conf.

nameserver 172.16.0.9
options edns0 trust-ad
search labo.local
```

設定確認
```
root@runner1:/etc# dig www.yahoo.co.jp +short
edge12.g.yimg.jp.
183.79.219.252
```

## ubuntu アップデート

```
$ sudo apt-get update -y
$ sudo apt-get upgrade -y
$ sudo apt-get install git curl gcc make kpartx
```

## LVMの設定

```
root@runner1:/etc# lsblk
NAME   MAJ:MIN RM   SIZE RO TYPE MOUNTPOINTS
loop0    7:0    0    62M  1 loop /snap/core20/1587
loop1    7:1    0  63.5M  1 loop /snap/core20/2015
loop2    7:2    0  79.9M  1 loop /snap/lxd/22923
loop3    7:3    0 111.9M  1 loop /snap/lxd/24322
loop4    7:4    0    47M  1 loop /snap/snapd/16292
vda    252:0    0    16G  0 disk 
├─vda1 252:1    0     1M  0 part 
└─vda2 252:2    0    16G  0 part /
vdb    252:16   0   100G  0 disk 
vdc    252:32   0   100G  0 disk 
vdd    252:48   0   100G  0 disk 
```


```
root@runner1:/etc# pvcreate /dev/vdc
  Physical volume "/dev/vdc" successfully created.
root@runner1:/etc# pvcreate /dev/vdd
  Physical volume "/dev/vdd" successfully created.
```

```
root@runner1:/etc# vgcreate vg1 /dev/vdc
  Volume group "vg1" successfully created
root@runner1:/etc# vgcreate vg2 /dev/vdd
  Volume group "vg2" successfully created
```

```
root@runner1:/etc# vgs
  VG  #PV #LV #SN Attr   VSize    VFree   
  vg1   1   0   0 wz--n- <100.00g <100.00g
  vg2   1   0   0 wz--n- <100.00g <100.00g
```


```
root@runner1:/etc# lvcreate --name lv01 --size 16GB vg1
  Logical volume "lv01" created.
root@runner1:/etc# lvs
  LV   VG  Attr       LSize  Pool Origin Data%  Meta%  Move Log Cpy%Sync Convert
  lv01 vg1 -wi-a----- 16.00g   
```


```
root@runner1:/etc# apt install nfs-common
```

```
# vi /etc/fstab
# cat /etc/fstab
hmc-nfs:/exports/nfs/golang /nfs nfs defaults 0 0
# mkdir /nfs
# mount /nfs
# df -h
Filesystem                   Size  Used Avail Use% Mounted on
tmpfs                        1.6G  1.4M  1.6G   1% /run
/dev/vda2                     16G  6.8G  8.2G  46% /
tmpfs                        7.9G     0  7.9G   0% /dev/shm
tmpfs                        5.0M     0  5.0M   0% /run/lock
tmpfs                        1.6G  4.0K  1.6G   1% /run/user/1000
tmpfs                        7.9G     0  7.9G   0% /run/qemu
hmc-nfs:/exports/nfs/golang  110G   91G   14G  87% /nfs
```

```
root@runner1:/home/ubuntu# mount -t nfs hmc-nfs:/backup /mnt
root@runner1:/home/ubuntu# df -h
Filesystem                   Size  Used Avail Use% Mounted on
tmpfs                        1.6G  1.4M  1.6G   1% /run
/dev/vda2                     16G  6.8G  8.2G  46% /
tmpfs                        7.9G     0  7.9G   0% /dev/shm
tmpfs                        5.0M     0  5.0M   0% /run/lock
tmpfs                        1.6G  4.0K  1.6G   1% /run/user/1000
tmpfs                        7.9G     0  7.9G   0% /run/qemu
hmc-nfs:/exports/nfs/golang  110G   91G   14G  87% /nfs
hmc-nfs:/backup              110G   51G   54G  49% /mnt
```

```
root@runner1:/# dd if=/mnt/lv03.img of=/dev/vg1/lv01 bs=4294967296
0+9 records in
0+9 records out
17179869184 bytes (17 GB, 16 GiB) copied, 157.197 s, 109 MB/s
```

```
# vi /etc/fstab
# cat /etc/fstab
...
hmc-nfs:/exports/nfs/golang /nfs nfs defaults 0 0
/dev/vdb /var ext4 defaults 0 0

# mkfs.ext4 /dev/vdb
# mkdir /var
# mount /var
# df -h
# tar xvf /mnt/var.tar 
# cd /
# rm -fr var.backup/
```


```
root@runner1:/# rm /etc/resolv.conf 
root@runner1:/# vi /etc/resolv.conf
cat /etc/resolv.conf
# from Ansible template
#
nameserver 172.16.0.9
options edns0 trust-ad
search labo.local
```
