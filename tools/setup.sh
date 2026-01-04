#!/bin/bash -e

#QCOW2POOL="/var/lib/libvirt/images"
QCOW2POOL="/var/lib/marmot/volumes"

if [ -d "${QCOW2POOL}" ]; then
  echo "${QCOW2POOL} ディレクトリは存在します。"
else
  echo "${QCOW2POOL} ディレクトリを作成します。"
  mkdir -p ${QCOW2POOL}
fi

echo "Ubuntu 22.04 (jammy) のcloud imageをダウンロードしてカスタマイズする"

if [ ${CI_ENVIRONMENT} = "true" ]; then
  curl -OL http://10.1.0.12/jammy-server-cloudimg-amd64.img
else
  curl -OL https://cloud-images.ubuntu.com/jammy/20251216/jammy-server-cloudimg-amd64.img
fi

echo "cloud imageのカスタマイズを行う"
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
      dhcp4: false
      dhcp6: false
    enp2s0:
      dhcp4: false
      dhcp6: false
    enp7s0:
      dhcp4: false
      dhcp6: false
    enp8s0:
      dhcp4: false
      dhcp6: false
'

echo "LVMボリュームを作成してcloud imageをコピーする"
lvcreate -L 2.2G -n lvos_temp -y vg1
modprobe nbd max_part=8
qemu-nbd --connect=/dev/nbd0 jammy-server-cloudimg-amd64.img
sleep 3
dd if=/dev/nbd0 of=/dev/vg1/lvos_temp bs=1M status=progress
sleep 3

echo "VM用のスナップショットを作成"
lvcreate -s -L 1G -n oslv -y /dev/vg1/lvos_temp

echo "VM用のデータボリューム作成"
lvcreate -L 1G -n lvdata -y vg1

echo "追加のデータ用ディスクイメージ作成"
qemu-img create -f qcow2 ${QCOW2POOL}/data-vol-1.qcow2 1G
qemu-img create -f qcow2 ${QCOW2POOL}/data-vol-2.qcow2 1G


echo "lxc用FSを作成、rsyncでコピーする"
lvcreate -L 4.0G -n temp01 -y vg1
mkfs.ext4 /dev/vg1/temp01
mkdir -p /mnt/src
mkdir -p /mnt/dst
mount /dev/vg1/temp01 /mnt/dst
mount /dev/nbd0p1 /mnt/src
rsync -auz /mnt/src/ /mnt/dst/
umount /mnt/src
umount /mnt/dst

qemu-nbd --disconnect /dev/nbd0

echo "lxc用のスナップショットを作成"
lvcreate -s -L 1G -n boot01 -y /dev/vg1/temp01


echo "ｑcow2イメージを移動"
mv jammy-server-cloudimg-amd64.img ${QCOW2POOL}/
chown libvirt-qemu:kvm ${QCOW2POOL}/jammy-server-cloudimg-amd64.img
chmod 644 ${QCOW2POOL}/jammy-server-cloudimg-amd64.img

echo "LXC用のrootfsを作成する"
mkdir -p /var/lib/lxc/rootfs/lxc-test-1
mount /dev/vg1/boot01 /var/lib/lxc/rootfs/lxc-test-1

echo "LXC用のデータファイルシステムを作成する"
mkdir -p /var/lib/lxc/shared-data

echo "LXC用のネットワーク設定を行う"
cat << EOF | sudo tee /var/lib/lxc/rootfs/lxc-test-1/etc/netplan/00-nic.yaml
network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      dhcp4: yes
      dhcp6: yes
EOF

echo "rootfs 内の bash を起動、初期設定を行う"
chroot /var/lib/lxc/rootfs/lxc-test-1 ssh-keygen -A
