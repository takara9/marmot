#!/bin/bash

virsh destroy vm-test-1
virsh undefine vm-test-1
rm -f /var/lib/libvirt/images/jammy-server-cloudimg-amd64.img
rm -f /var/lib/marmot/volumes/jammy-server-cloudimg-amd64.img

qemu-nbd --disconnect /dev/nbd0
lvremove -y /dev/vg1/lvos_temp
lvremove -y /dev/vg1/oslv

virsh -c lxc:///system destroy lxc-test-1
virsh -c lxc:///system undefine lxc-test-1

umount /var/lib/lxc/rootfs/lxc-test-1
lvremove -y /dev/vg1/temp01
lvremove -y /dev/vg1/boot01

rm -rf /var/lib/lxc/rootfs