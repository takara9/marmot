## Ubuntu qcow2イメージをダウンロードして利用する。

https://askubuntu.com/questions/451673/default-username-password-for-ubuntu-cloud-image



ダウンロードサイト: https://cloud-images.ubuntu.com/


## ダウンロード

```
wget https://cloud-images.ubuntu.com/daily/server/jammy/current/jammy-server-cloudimg-amd64.img
```

## コマンドのインストール

```
sudo apt install libguestfs-tools
```


## パスワードの設定

```
$ sudo virt-customize -a jammy-server-cloudimg-amd64.img --root-password password:ubuntu
```

## qcow2形式に変換し、ディスクサイズを20GBに拡張する

```
# mv jammy-server-cloudimg-amd64.img jammy-server-cloudimg-amd64.qcow2.original
# qemu-img convert -f qcow2 -O qcow2 jammy-server-cloudimg-amd64.qcow2.original jammy-bootable.qcow2
# qemu-img resize jammy-bootable.qcow2 +18G
```

## cidata.isoの作成

user-dataを編集する

```console
cat << EOF > user-data
#cloud-config
users:
  - name: ubuntu
    ssh_authorized_keys:
      - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDC7ciYRXg20phLiWN4Dq4JNs5pWsMU/8sHZKesREjf9OPAyE8fegP2XkIy7ZFAV1oM+TeDQvVVrIuziJWcuoXf9/tnLLOt82zKJ89EcSUBuqERuPUrp2hqD52ff/yOFGcLGMSjtxjTZLQy40ZBUgBM8cbexqQY92mo0A9MKMbHNve0Y5FhBb2nq8EEml8qbE98hvfxScmuLCAD8OUfdgQeLqIHCCjy2IcxtazChPLyEBbcLnRGMZUFnNO8lEt8RWAw5HnZ/fI70335REQ2zctiSBWatBDOYE8anvAlek5m18BCyahxfeTxe27nz+1qslqsNtjCaJs1kWl+8u8QT8/n takara9@github/41478507
    sudo: ALL=(ALL) NOPASSWD:ALL
network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      dhcp4: true
EOF
```

## meta-dataの作成

```
touch meta-data
```

## cidata.isoを作成

```
genisoimage -output cidata.iso -volid cidata -joliet -rock user-data meta-data
```

## VMの起動

```
qemu-system-x86_64 \
  -enable-kvm \
  -m 2048 \
  -cpu host \
  -smp 2 \
  -drive if=virtio,file=jammy-bootable.qcow2,format=qcow2,index=0,media=disk \
  -drive file=cidata.iso,if=ide,format=raw,index=1,media=cdrom \
  -boot d \
  -device virtio-net-pci,netdev=net0 \
  -netdev user,id=net0,hostfwd=tcp::2222-:22 \
  -nographic
```

## sshログイン

```
ssh -p 2222 ubuntu@localhost
```

## シャットダウン

```
sudo systemctl poweroff
```

## 起動

起動した画面がコンソールになる
```
qemu-system-x86_64 \
  -enable-kvm \
  -m 2048 \
  -cpu host \
  -smp 2 \
  -drive if=virtio,file=jammy-bootable.qcow2,format=qcow2,index=0,media=disk \
  -boot d \
  -device virtio-net-pci,netdev=net0 \
  -netdev user,id=net0,hostfwd=tcp::2222-:22 \
  -nographic
```

デーモンとして起動
```
qemu-system-x86_64 \
  -enable-kvm \
  -m 2048 \
  -cpu host \
  -smp 2 \
  -name ubuntu-jammy-cloud \
  -drive if=virtio,file=jammy-bootable.qcow2,format=qcow2,index=0,media=disk \
  -boot d \
  -device virtio-net-pci,netdev=net0 \
  -netdev user,id=net0,hostfwd=tcp::2222-:22 \
  -nographic
  -daemonize  # ← これを追加
```

## TODO
ネットワークの設定を改善する必要がる


qemu-system-x86_64 \
  -enable-kvm \
  -m 2048 \
  -cpu host \
  -smp 2 \
  -name rocky-linux-cloud \
  -drive if=virtio,file=rocky-bootable.qcow2,format=qcow2,index=0,media=disk \
  -drive file=cidata.iso,if=ide,format=raw,index=1,media=cdrom \
  -boot d \
  -device virtio-net-pci,netdev=net0 \
  -netdev user,id=net0,hostfwd=tcp::2225-:22 \
  -nographic