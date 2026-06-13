# Apline Linux の Cloud Image

curl -OL https://dl-cdn.alpinelinux.org/alpine/v3.23/releases/cloud/generic_alpine-3.23.0-x86_64-bios-cloudinit-metal-r0.qcow2

$ sudo modprobe nbd max_part=8
$ sudo qemu-nbd -c /dev/nbd0 generic_alpine-3.23.0-x86_64-bios-cloudinit-metal-r0.qcow2 
$ sudo lsblk /dev/nbd0
NAME MAJ:MIN RM  SIZE RO TYPE MOUNTPOINTS
nbd0  43:0    0  368M  0 disk 


$ sudo fdisk -l /dev/nbd0
ディスク /dev/nbd0: 368 MiB, 385875968 バイト, 753664 セクタ
単位: セクタ (1 * 512 = 512 バイト)
セクタサイズ (論理 / 物理): 512 バイト / 512 バイト
I/O サイズ (最小 / 推奨): 512 バイト / 131072 バイト
ディスクラベルのタイプ: dos
ディスク識別子: 0x20ac7dda

デバイス    起動   開始位置   終了位置     セクタ サイズ Id タイプ
/dev/nbd0p1      3224498923 3657370039  432871117 206.4G  7 HPFS/NTFS/exFAT
/dev/nbd0p2      3272020941 5225480974 1953460034 931.5G 16 隠し FAT16
/dev/nbd0p3               0          0          0     0B 6f 不明
/dev/nbd0p4        50200576  974536369  924335794 440.8G  0 空

パーティション情報の項目がディスクの順序と一致しません。



---
# マウント
sudo modprobe nbd max_part=8
sudo qemu-nbd --connect=/dev/nbd0 generic_alpine-3.23.0-x86_64-bios-cloudinit-metal-r0.qcow2 
sudo mount /dev/nbd0 /mnt

# パスワードをセット（chroot で）
sudo chroot /mnt /bin/ash -c "echo 'alpine:alpine' | chpasswd"

# または shadow を直接書き換え（パスワードなしで root ログイン可能にする）
sudo sed -i 's/^root:!*/root:/' /mnt/etc/shadow
sudo sed -i 's/^alpine:!*/alpine:/' /mnt/etc/shadow

# アンマウント
sudo umount /mnt
sudo qemu-nbd --disconnect /dev/nbd0


---

