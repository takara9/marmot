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


# 2Gをコピーする
lvcreate -L 2.2G -n lvos_test vg1
modprobe nbd max_part=8
qemu-nbd --connect=/dev/nbd0 jammy-server-cloudimg-amd64.img
dd if=/dev/nbd0 of=/dev/vg1/lvos_test bs=1M status=progress
qemu-nbd --disconnect /dev/nbd0


# ｑcow2イメージを移動
mv jammy-server-cloudimg-amd64.img /var/lib/libvirt/images/
chown libvirt-qemu:kvm /var/lib/libvirt/images/jammy-server-cloudimg-amd64.img
chmod 644 /var/lib/libvirt/images/jammy-server-cloudimg-amd64.img

