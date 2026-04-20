#!/bin/bash -e

echo "Ubuntu 24.04 (noble) のcloud imageをダウンロードしてカスタマイズする"
QCOW2POOL="/var/lib/marmot/volumes"
IMAGE="ubuntu-24.04-server-cloudimg-amd64.img"
IMAGE_TEMPLATE="ubuntu-24.04-template.qcow2"

if [ -d "${QCOW2POOL}" ]; then
  echo "${QCOW2POOL} ディレクトリは存在します。"
else
  echo "${QCOW2POOL} ディレクトリを作成します。"
  mkdir -p ${QCOW2POOL}
fi
cd ${QCOW2POOL}

echo "http://hmc/${IMAGE} にアクセスしてダウンロードの成否を確認する"
curl -I http://hmc/${IMAGE} |tee  download.log
if grep -q "200 OK" download.log; then
  echo "ダウンロード可能: http://hmc/${IMAGE}"
  curl -OL http://hmc/${IMAGE}
else
  echo "ダウンロード不可: http://hmc/${IMAGE}。インターネットからcloud imageをダウンロードします。"
  curl -OL https://cloud-images.ubuntu.com/releases/noble/release/${IMAGE}
fi
rm -f download.log

echo "cloud imageのカスタマイズを行う"
cp ${IMAGE} ${IMAGE_TEMPLATE}
virt-customize -a ${IMAGE_TEMPLATE} \
  --root-password password:ubuntu \
  --edit '/etc/ssh/sshd_config: s/^#?PermitRootLogin.*/PermitRootLogin yes/' \
  --edit '/etc/ssh/sshd_config: s/^#?PasswordAuthentication.*/PasswordAuthentication yes/' \
  --run-command 'rm /etc/ssh/sshd_config.d/60-cloudimg-settings.conf' \
  --run-command "ssh-keygen -A" \
  --run-command "systemctl enable ssh" \
  --run-command "systemctl restart ssh" \
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
echo "imageのリサイズとパーティションの拡張を実施する"
modprobe nbd max_part=8
qemu-img resize ${IMAGE_TEMPLATE} 16G
qemu-nbd -c /dev/nbd1 ${IMAGE_TEMPLATE}
sleep 3
lsblk
parted /dev/nbd1 --fix --script 'resizepart 1 17.2G quit'
sleep 3
e2fsck -f /dev/nbd1p1 -y 
resize2fs /dev/nbd1p1
#
qemu-nbd -d /dev/nbd1 
sleep 3
qemu-img info ${IMAGE_TEMPLATE} 


echo "ｑcow2イメージを移動"
chown libvirt-qemu:kvm ${QCOW2POOL}/${IMAGE_TEMPLATE}
chmod 644 ${QCOW2POOL}/${IMAGE_TEMPLATE}



echo "LVMボリュームを作成してcloud imageをコピーする"
lvcreate -L 16G -n lvos_temp -y vg1
modprobe nbd max_part=8
qemu-nbd --connect=/dev/nbd2 ${IMAGE}
sleep 3
dd if=/dev/nbd2 of=/dev/vg1/lvos_temp bs=1M status=progress
sleep 3

echo "VM用のスナップショットを作成"
lvcreate -s -L 1G -n oslv -y /dev/vg1/lvos_temp

echo "VM用のデータボリューム作成"
lvcreate -L 1G -n lvdata -y vg1

echo "追加のデータ用ディスクイメージ作成"
qemu-img create -f qcow2 ${QCOW2POOL}/data-vol-1.qcow2 1G
qemu-img create -f qcow2 ${QCOW2POOL}/data-vol-2.qcow2 1G

qemu-nbd --disconnect /dev/nbd2


echo "lxc用FSを作成、rsyncでコピーする"
#lvcreate -L 4.0G -n temp01 -y vg1
#mkfs.ext4 /dev/vg1/temp01
#mkdir -p /mnt/src
#mkdir -p /mnt/dst
#mount /dev/vg1/temp01 /mnt/dst
#mount /dev/nbd0p1 /mnt/src
#rsync -auz /mnt/src/ /mnt/dst/
#umount /mnt/src
#umount /mnt/dst

echo "lxc用のスナップショットを作成"
#lvcreate -s -L 1G -n boot01 -y /dev/vg1/temp01

echo "LXC用のrootfsを作成する"
#mkdir -p /var/lib/lxc/rootfs/lxc-test-1
#mount /dev/vg1/boot01 /var/lib/lxc/rootfs/lxc-test-1

echo "LXC用のデータファイルシステムを作成する"
#mkdir -p /var/lib/lxc/shared-data

echo "LXC用のネットワーク設定を行う"
#cat << EOF | sudo tee /var/lib/lxc/rootfs/lxc-test-1/etc/netplan/00-nic.yaml
#network:
#  version: 2
#  renderer: networkd
#  ethernets:
#    eth0:
#      dhcp4: yes
#      dhcp6: yes
#EOF

echo "LXC用 rootfs 内の bash を起動、初期設定を行う"
#chroot /var/lib/lxc/rootfs/lxc-test-1 ssh-keygen -A

echo "作業ファイルのクリーンナップ"
