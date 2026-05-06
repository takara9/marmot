# OSテンプレートの準備


## 他のサーバーからOSテンプレートのイメージを取り出す

```
ubuntu@hv1:~$ sudo lvs
  LV       VG  Attr       LSize  Pool Origin Data%  Meta%  Move Log Cpy%Sync Convert
  data0109 vg1 -wi-ao----  2.00g                                                    
  data0110 vg1 -wi-ao----  2.00g                                                    
  data0111 vg1 -wi-ao----  2.00g                                                    
  data0112 vg1 -wi-ao----  2.00g                                                    
  data0113 vg1 -wi-ao----  2.00g                                                    
  lv01     vg1 owi-a-s--- 16.00g                                                    
  lv02     vg1 -wi-a----- 16.00g                                                    
  lv03     vg1 owi-a-s--- 16.00g               

# time dd if=/dev/vg1/lv02 of=/home/ubuntu/lv02.img bs=8589934592
0+9 records in
0+9 records out
17179869184 bytes (17 GB, 16 GiB) copied, 699.954 s, 24.5 MB/s

real	11m40.071s
user	0m0.000s
sys	0m21.672s

```


## ディスクイメージをインポートする

```
$ sudo lvcreate --name lv01 --size 16GB vg1
  Logical volume "lv01" created.
$ sudo dd if=lv01.img of=/dev/vg1/lv01 bs=8589934592
0+9 records in
0+9 records out
17179869184 bytes (17 GB, 16 GiB) copied, 73.4845 s, 234 MB/s

$ sudo lvcreate --name lv02 --size 16GB vg1
  Logical volume "lv02" created.
$ sudo dd if=lv02.img of=/dev/vg1/lv02 bs=8589934592
0+9 records in
0+9 records out
17179869184 bytes (17 GB, 16 GiB) copied, 73.3119 s, 234 MB/s
```
