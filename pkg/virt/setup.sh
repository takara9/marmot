#!/bin/bash -e

curl -OL https://cloud-images.ubuntu.com/jammy/20251216/jammy-server-cloudimg-amd64.img

virt-customize -a jammy-server-cloudimg-amd64.img --root-password password:ubuntu

virt-customize -a jammy-server-cloudimg-amd64.img \
  --edit '/etc/ssh/sshd_config: s/^#?PermitRootLogin.*/PermitRootLogin yes/' \
  --edit '/etc/ssh/sshd_config: s/^#?PasswordAuthentication.*/PasswordAuthentication yes/' \
  --run-command 'rm /etc/ssh/sshd_config.d/60-cloudimg-settings.conf'

virt-customize -a jammy-server-cloudimg-amd64.img \
  --run-command "ssh-keygen -A" \
  --run-command "systemctl enable ssh" \
  --run-command "systemctl restart ssh"
  

virt-customize -a jammy-server-cloudimg-amd64.img \
  --write /etc/netplan/00-nic.yaml:'network:
  version: 2
  ethernets:
    enp1s0:
      dhcp4: true'


# VM用 2Gをコピーする
lvcreate -L 2.2G -n lvos_temp vg1
modprobe nbd max_part=8
qemu-nbd --connect=/dev/nbd0 jammy-server-cloudimg-amd64.img
sleep 3
dd if=/dev/nbd0 of=/dev/vg1/lvos_temp bs=1M status=progress
sleep 3
# VM用のスナップショットを作成
lvcreate -s -L 1G -n oslv /dev/vg1/lvos_temp
# VM用のデータボリューム作成
lvcreate -L 1G -n lvdata vg1
# 追加ディスクイメージ作成
qemu-img create -f qcow2 /var/lib/marmot/volumes/data-vol-1.qcow2 1G
qemu-img create -f qcow2 /var/lib/marmot/volumes/data-vol-2.qcow2 1G


# lxc用FSを作成、rsyncでコピーする
lvcreate -L 4.0G -n temp01 vg1
mkfs.ext4 /dev/vg1/temp01
mkdir -p /mnt/src
mkdir -p /mnt/dst
mount /dev/vg1/temp01 /mnt/dst
mount /dev/nbd0p1 /mnt/src
rsync -auz /mnt/src/ /mnt/dst/
umount /mnt/src
umount /mnt/dst

qemu-nbd --disconnect /dev/nbd0

# lxc用のスナップショットを作成
lvcreate -s -L 1G -n boot01 /dev/vg1/temp01


# ｑcow2イメージを移動
mv jammy-server-cloudimg-amd64.img /var/lib/marmot/volumes/
chown libvirt-qemu:kvm /var/lib/marmot/volumes/jammy-server-cloudimg-amd64.img
chmod 644 /var/lib/marmot/volumes/jammy-server-cloudimg-amd64.img



## LXC用のrootfsを作成する場合
# ディレクトリ作成
mkdir -p /var/lib/lxc/rootfs/lxc-test-1
mount /dev/vg1/boot01 /var/lib/lxc/rootfs/lxc-test-1


# Ubuntu 22.04 (jammy) をインストールする場合
#sudo debootstrap jammy /var/lib/lxc/rootfs/lxc-test-1 http://archive.ubuntu.com/ubuntu/

cat << EOF | sudo tee /var/lib/lxc/rootfs/lxc-test-1/etc/netplan/00-nic.yaml
network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      dhcp4: yes
      dhcp6: yes
EOF


# rootfs 内の bash を起動
chroot /var/lib/lxc/rootfs/lxc-test-1 ssh-keygen -A
#chroot /var/lib/lxc/rootfs/lxc-test-1 systemctl enable ssh

