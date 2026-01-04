#!/bin/bash

echo "VM一覧:"
virsh list --name --all > domain_list.txt

echo "LXC一覧:"
virsh -c lxc:///system list --name --all > lxc_domain_list.txt

while read domain; do
    if [ -n "$domain" ]; then
        echo "Destroying and undefining domain: $domain"
        virsh destroy "$domain"
        virsh undefine "$domain"
    fi
done < domain_list.txt

qemu-nbd --disconnect /dev/nbd0

rm -f /var/lib/libvirt/images/jammy-server-cloudimg-amd64.img

umount /var/lib/lxc/rootfs/lxc-test-1
rm -rf /var/lib/lxc/rootfs
                              
lvs --reportformat json | tee  lv_list.json
cat lv_list.json | /usr/bin/jq -r '.report[].lv[] | .vg_name + "/" + .lv_name' | sed 's/vg1\/lv01//g' | sed '/^$/d' > lv_to_remove.txt

cat lv_to_remove.txt
while read lv; do
    lvremove -y /dev/$lv
done < lv_to_remove.txt

ids=$(docker ps -q); [ -n "$ids" ] && docker kill $ids
ids=$(docker ps -aq); [ -n "$ids" ] && docker rm $ids

exit 0
