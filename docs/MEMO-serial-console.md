# Ubuntu を Serial Console 化する方法

参考資料: https://help.ubuntu.com/community/SerialConsoleHowto



1) OSストレージを作成

```
# cd /var/lib/marmot/images
# qemu-img create -f qcow2 ubuntu22.04.qcow2 25G
```

2) OSを起動後に、シリアルコンソールの設定を実施 

`vi /etc/default/grub`

```
# If you change this file, run 'update-grub' afterwards to update
# /boot/grub/grub.cfg.
 
GRUB_DEFAULT=0
GRUB_TIMEOUT=1
GRUB_DISTRIBUTOR=`lsb_release -i -s 2> /dev/null || echo Debian`
GRUB_CMDLINE_LINUX="console=tty0 console=ttyS0,115200n8"
 
# Uncomment to disable graphical terminal (grub-pc only)
GRUB_TERMINAL=serial
GRUB_SERIAL_COMMAND="serial --speed=115200 --unit=0 --word=8 --parity=no --stop=1"
 
# The resolution used on graphical terminal
# note that you can use only modes which your graphic card supports via VBE
# you can see them in real GRUB with the command `vbeinfo'
#GRUB_GFXMODE=640x480
 
# Uncomment if you don't want GRUB to pass "root=UUID=xxx" parameter to Linux
#GRUB_DISABLE_LINUX_UUID=true
```

2) update grubのためのコマンドを実行

```
# update-grub
```


## qcow2 のスナップショット

参考資料: https://takuya-1st.hatenablog.jp/entry/2022/04/22/165135

