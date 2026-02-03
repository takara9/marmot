#!/bin/bash

#QCOW2POOL="/var/lib/libvirt/images"
QCOW2POOL="/var/lib/marmot/volumes"

echo "VM一覧:"
virsh list --name --all |tee domain_list.txt

while read domain; do
    if [ -n "$domain" ]; then
        echo "Destroying and undefining domain: $domain"
        virsh destroy "$domain"
        virsh undefine "$domain"
    fi
done < domain_list.txt

echo "LXC一覧:"
virsh -c lxc:///system list --name --all |tee lxc_domain_list.txt
while read domain; do
    if [ -n "$domain" ]; then
        echo "Destroying and undefining domain: $domain"
        virsh -c lxc:///system shutdown "$domain"
        sleep 2
        virsh -c lxc:///system undefine "$domain"
    fi
done < lxc_domain_list.txt

qemu-nbd --disconnect /dev/nbd0
kpartx -d /dev/mapper/vg1-lvos_test


rm -f ${QCOW2POOL}/jammy-server-cloudimg-amd64.img

umount /var/lib/lxc/rootfs/lxc-test-1
rm -rf /var/lib/lxc/rootfs
rm -fr /var/lib/lxc/shared-data
                              
lvs --reportformat json | tee  lv_list.json
cat lv_list.json | /usr/bin/jq -r '.report[].lv[] | .vg_name + "/" + .lv_name' | sed 's/vg1\/lv01//g' | sed 's/vg1\/lv02//g' | sed 's/vg1\/lv03//g' | sed '/^$/d' > lv_to_remove.txt

while read lv; do
    lvremove -y /dev/$lv
done < lv_to_remove.txt

ids=$(docker ps -q); [ -n "$ids" ] && docker kill $ids
ids=$(docker ps -aq); [ -n "$ids" ] && docker rm $ids

rm -f domain_list.txt
rm -f lxc_domain_list.txt
rm -f lv_list.json
rm -f lv_to_remove.txt

exit 0
