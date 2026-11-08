1. Ubuntu24.04 Destktopをイントール
1. IPアドレスを割り当て
2. apt-get update
3. apt install openssh-server
4. systemctl stop NetworkManager
5. systemctl disable NetworkManager
systemctl enable systemd-networkd
6. netplan を設定
7. .ssh/autuhentication の編集、 ユーザーとRoot
8. sudoers.d

mkfs.ext4 /dev/nvme0n1 
blkid /dev/nvme0n1
mkdir /var/lib/marmot
vi /etc/fstab

root@m1:~# cat /etc/fstab 
/dev/disk/by-uuid/8d978521-2b07-4619-8f4d-9d4f8baf6d0a / ext4 defaults 0 1
/dev/disk/by-uuid/D0B7-37E7 /boot/efi vfat defaults 0 1
/swap.img	none	swap	sw	0	0
UUID="63732554-1c51-471f-b0db-70ac96aa3324" /var/lib/marmot ext4 defaults 0 1


apt install lvm2


root@m3:/etc/marmot# maadm cluster join eb257a62-6de8-467b-a598-49bfa350759d --master-node 10.1.0.11
Error: HOSTID 007f0101 は、クラスタ内のノード "m1" と重複しています（/etc/hostid を変更してください）
Usage:
  maadm cluster join CLUSTER-UUID [flags]

Flags:
      --dry-run                     変更内容を表示するだけで、何も変更しない
      --force                       確認プロンプトを省略する
  -h, --help                        help for join
      --master-node string          マスターノードの IP アドレス（必須。etcd のポートは 2379）
      --underlay-interface string   ノード間のオーバーレイに使うインターフェース（既定: br0）

Global Flags:
  -o, --output string   Text style output (default "text")

{"time":"2026-11-08T14:55:19.091870949+09:00","level":"ERROR","source":{"function":"github.com/takara9/marmot/cmd/maadm/cmd.Execute","file":"/home/ubuntu/marmot/cmd/maadm/cmd/root.go","line":25},"msg":"Error exit","err":"HOSTID 007f0101 は、クラスタ内のノード \"m1\" と重複しています（/etc/hostid を変更してください）"}
