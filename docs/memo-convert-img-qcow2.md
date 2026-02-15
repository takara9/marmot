## imgのダウンロードから qcow2 テンプレートの作成

ルートファイルシステムを拡大

```
$ curl -OL https://cloud-images.ubuntu.com/jammy/20251216/jammy-server-cloudimg-amd64.img
```



```
$ sudo qemu-img convert -f qcow2 -O qcow2 jammy-server-cloudimg-amd64.img jammy-server-cloudimg-amd64.qcow2
$ sudo virt-customize -a jammy-server-cloudimg-amd64.qcow2 \
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
$ sudo qemu-img resize jammy-server-cloudimg-amd64.qcow2 16G
$ sudo qemu-nbd -c /dev/nbd0 jammy-server-cloudimg-amd64.qcow2 
$ sudo parted /dev/nbd0 --fix --script 'resizepart 1 17.2G quit'
$ sudo e2fsck -f /dev/nbd0p1 
$ sudo resize2fs /dev/nbd0p1
$ sudo qemu-nbd -d /dev/nbd0 
```

$ ./configure --disable-device-mapper --without-readline 

$ sudo apt-get install uuid-dev libdevmapper 

curl -OL https://ftp.gnu.org/gnu/parted/parted-3.6.tar.xz
xz -dv parted-3.6.tar.xz
