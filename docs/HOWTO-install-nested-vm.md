# 仮想マシンの上にmarmotをインストールする

このドキュメントは、Ubuntu Desktop の 仮想マシンマネージャーで管理する仮想マシン上に、Ubuntu Server 24.04 をインストールして、その上で Marmotをインストール手順です。 Marmotは、Nested VM に対応しているので、仮想マシン上で動作させることもできます。

## Ubuntu Server 24.04 のインストールイメージの入手

[Ubuntu releases](https://releases.ubuntu.com/24.04.5/) のWebサイトに登録されたインストールイメージの中から、`ubuntu-24.04.5-live-server-amd64.iso`を選んでダウンロードします。[このリンク](https://releases.ubuntu.com/24.04.5/ubuntu-24.04.5-live-server-amd64.iso) をクリックするとダウンロードが始まります。

## 仮想マシンの起動とインストール準備

仮想マシンマネージャーを利用して、Ubuntu Desktop 上で、仮想マシンを管理できます。この機能の詳細は、Ubuntu Server Documentation の [Virtual Machine Manager](https://ubuntu.com/server/docs/how-to/virtualisation/Virtual-machine-manager/
) のパートに説明があります。`Ubuntu Virtual Machine Manager の使い方`で検索すれば、日本語の解説記事も沢山でヒットしますので、ご参考にしてください。

ダウンロードしたインストールメディアを利用して、Ubuntu 24.04 が稼働する仮想マシンを起動します。

仮想マシンマネージャーで [ファイル]->[新しい仮想マシン]を選択します。
![](pictures/Screenshot-101.png)

「新しい仮想マシン」のウィンドが表示されるので、以下を選んで、「次へ」をクリックします。
- 接続(O): QEMU/KVM を選択
- ローカルのインストールメディア(ISOイメージまたはCD-ROMドライブ)(L)　を選択

![](pictures/Screenshot-102.png)


前述でダウンロードしたISOイメージ・ファイルを選択します。
![](pictures/Screenshot-103.png)

メモリとCPUの設定は、特に決まりはありませんが、以下くらいを割り当てると、Marmotも動作して、ホストマシンの Ubuntu Desktop の動作用資源も確保されると思います。動かしてみて調整が必要かもしれません。

- メモリ(M): 6144 (6Gバイト)
- CPU(P): 4 (コア)

![](pictures/Screenshot-104.png)

次に仮想マシン用のディスクイメージです。40Gくらいあれば、Marmot上で 数台の仮想サーバーを起動できると思います。
![](pictures/Screenshot-105.png)

起動する仮想マシンのホスト名です。見てホストの利用目的が判る名前にしておきます。
![](pictures/Screenshot-106.png)

ネットワークの選択です。仮想ネットワークを `default`になっていると思います。
Marmotで起動した仮想マシンに、他のWindowsやMacOSのパソコンからアクセスできる様にするため、`host-bridge` という名前でブリッジネットワークを作成します。

好みのエディタで、以下の内容のxmlファイルを作成します。

```yaml
<network>
  <name>host-bridge</name>
  <forward mode='bridge'/>
  <bridge name='br0'/>
</network>
```

次のコマンドで、`host-bridge` という名前でブリッジネットワークを作成します。
```bash
$ sudo virsh net-define host-bridge.xml 
$ sudo virsh net-start host-bridge
$ sudo virsh net-autostart host-bridge
```

Marmotが起動する仮想マシンを、この`host-bridge`に接続することで、Marmotで起動したマシンへ、自宅のLAN上のパソコンからアクセスできる様になります。

![](pictures/Screenshot-107.png)


## Ubuntu Server 24.04 のインストール

ここからは通常の Ubuntu Linux のインストールと同じです。
`Try or Install Ubuntu Server` を選択
![](pictures/Screenshot-108.png)

ここは`English`を選択するのが無難です。
![](pictures/Screenshot-109.png)

キーボードは、環境に合わせて選択します。多分、`Japanese`の人が多数だと思います。
![](pictures/Screenshot-110.png)

次は、`Ubuntu Server (minimal)` を選ぶと使わないプログラムでディスクが占有されるのを減らす事ができます。
![](pictures/Screenshot-111.png)

自宅LANのDHCPから、ネットワークアドレスを取得していると思います。固定のIPアドレスを割り当てるのが、お勧めですが、DHCPで割り当てられたアドレスを覚えておいて、アクセスすることも可能です。
![](pictures/Screenshot-112.png)

デフォルトのまま進めます。
![](pictures/Screenshot-113.png)

LVMを利用する必要は無いので、チェックを外しておきます。
![](pictures/Screenshot-115.png)

デフォルトで進めます。
![](pictures/Screenshot-116.png)

問題なければ、続けます。
![](pictures/Screenshot-117.png)

筆者の好みでは、内部的に使用するUbuntuサーバーは、全て同じ `ubuntu` にしています。読者の好みに変更して問題ありません。
![](pictures/Screenshot-118.png)

スキップにチェックを入れます。
![](pictures/Screenshot-119.png)

OpenSSH Serverをインストールしないと、自宅LAN上からアクセスできないので、チェックを入れます。
SSH鍵は、普段利用するものを使えば良いです。もしGitHub公開しているものがあれば、それを利用するのが良いでしょう。
![](pictures/Screenshot-120.png)

選択する必要はありません。そのまま、次へ進みます。
![](pictures/Screenshot-121.png)

インストールが終わるまで待ち、再起動します。
![](pictures/Screenshot-122.png)

## Marmot のインストール
ここからは、ssh で、起動した仮想マシンにログインして、操作します。

`cd /tmp` で移動して作業します。

[https://github.com/takara9/marmot/releases](https://github.com/takara9/marmot/releases)から、`curl -OL https://github.com/takara9/marmot/releases/download/v0.38.0/marmot_v0.38.0_amd64.deb` でダウンロードします。
![](pictures/Screenshot-123.png)

`apt install -y ./marmot_v0.38.0_amd64.deb` でインストールを実行します。
![](pictures/Screenshot-124.png)

途中、`Allow HTTP tunnels...` と尋ねてくる箇所があるので、`no`として、続行します。
![](pictures/Screenshot-125.png)

インストールが完了すると、以下の様に表示されます。
![](pictures/Screenshot-126.png)

少し `mactl version` コマンドを実行して、動作を確かめます。そして、初期パスワードをセットします。
![](pictures/Screenshot-127.png)

`mactl get images` で利用可能な クラウドイメージの準備状況を確認します。
これで、`AVAILABLE`となれば、このマシンイメージを利用して、仮想マシンが起動できます。
![](pictures/Screenshot-128.png)

## Marmot で仮想マシンの起動

Marmotのマニフェスト集のリポジトリを `git clone` します。

```bash
$ git clone https://github.com/takara9/marmot-manifests
```

マニフェストが、クローンされるので、その中から、`02_host-bridge_server`　を利用して仮想マシンを起動します。
```bash
$ cd marmot-manifests
$ ls
01_air-gapped_server                     40-expand-boot-volume                  60-micro-services
02_host-bridge_server                    41-volume-attach                       61_gitlab-ce
03_server-as-dhcp-client                 42-volume-create-when-creating-server  62_ca-acme
04_server-on-private-network             43-lvm-volume                          63-ca-labo-local
05_connect-server-on-private-by-openvpn  44-iscsi-vol                           64-ansible-ws
06_application-loadbalancer              45-ceph-block-storage                  65-http-server-nginx
07_network-loadbalancer                  50-ansible-gcc-make-golang             66-http-server-apache
08_update-server-spec                    51-ansible-docker                      67-http-server-iis
10-change-user-password                  52-ansible-kubernetes                  68-vault-single
11-ssh-login-with-key                    53-ansible-dify                        69-rockey-nginx
12-ssh-login-with-github-key             54-ansible-webserver                   70-mke-simple
20-save-machine-image                    55-ansible-app-server-php              COMMAND_REFERENCE.md
21-image-export-import                   56-ceph-single-node                    README.md
22-import-cloud-image                    57-setup-with-node-exporter-alloy      TUTORIAL.md
30_server-on-private-net                 58-minikube
31_server_net                            59-ca-and-dns
```

このディレクトリを除いています。サーバーをデプロイするためのYAMLと SSH鍵があるのが判ります。
```bash
$ cd 02_host-bridge_server
$ ls -la
合計 48
drwxrwxr-x  3 ubuntu ubuntu 4096 Oct 26 11:53 .
drwxrwxr-x 47 ubuntu ubuntu 4096 Nov  8 06:27 ..
-rw-rw-r--  1 ubuntu ubuntu 4022 Oct 26 11:53 README.md
-rw-------  1 ubuntu ubuntu  411 Sep 24 05:09 id_ed25519
-rw-rw-r--  1 ubuntu ubuntu   96 Sep 24 05:09 id_ed25519.pub
-rw-rw-r--  1 ubuntu ubuntu 2590 Oct 26 11:53 id_rsa
-rw-rw-r--  1 ubuntu ubuntu  564 Oct 26 11:53 id_rsa.pub
drwxrwxr-x  2 ubuntu ubuntu 4096 Sep 24 05:09 image
-rw-rw-r--  1 ubuntu ubuntu  457 Oct 26 11:53 srv02-1.yaml
-rw-rw-r--  1 ubuntu ubuntu  921 Oct 26 11:53 srv02-2.yaml
-rw-rw-r--  1 ubuntu ubuntu 1060 Oct 26 11:53 srv02-3.yaml
-rw-rw-r--  1 ubuntu ubuntu  350 Oct 26 11:53 srv02-4.yaml
```

`srv02-1.yaml`を開いて中身を見てみます。 KubernetesのYAMLと似ていますね。
```bash
$ cat srv02-1.yaml 
apiVersion: v1
kind: Server
metadata:
    name: srv02-1
    comment: ホスト・ブリッジ接続の仮想サーバー、IPアドレス自動割り当て ED25519公開鍵利用
spec:
    cpu: 1
    memory: 1024
    mmImage: ubuntu24.04
    auth:
        publicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIP6CPvNHHeF8rHScC7vrq7HMqTPQJcl08atMSqHk/gSy For marmot VMs"
        # users:
        # - ubuntu
    networkInterface:
        - networkname: host-bridge
```

この YAML を `mactl create -f` でサーバーをデプロしてみます。１分程度で起動しました。
```bash
$ mactl create -f srv02-1.yaml 
リソースの作成要求が受け入れられました。ID: 6b75e

$ mactl get server
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE
----             ----          ------        ---  -------  ----------       -------          ---
srv02-1          ubuntu        PROVISIONING  1    1024     192.168.1.200    host-bridge      6s

$ mactl get server
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE
----             ----          ------        ---  -------  ----------       -------          ---
srv02-1          ubuntu        RUNNING       1    1024     192.168.1.200    host-bridge      1m
```

秘密鍵を使って、サーバーにログインするには、`mactl ssh -i ./id_ed25519 srv02-1` で良いです。IPアドレスを覚える必要はありません。

```bash
$ mactl ssh -i ./id_ed25519 srv02-1
The authenticity of host '192.168.1.200 (192.168.1.200)' can't be established.
ED25519 key fingerprint is SHA256:a6u50f00vAyAUN5BDVPUdIqO4aKYuJv1VrbonB8TLPM.
This key is not known by any other names.
Are you sure you want to continue connecting (yes/no/[fingerprint])? yes
Warning: Permanently added '192.168.1.200' (ED25519) to the list of known hosts.
Welcome to Ubuntu 24.04.5 LTS (GNU/Linux 6.8.0-147-generic x86_64)

 * Documentation:  https://help.ubuntu.com
 * Management:     https://landscape.canonical.com
 * Support:        https://ubuntu.com/pro
```

## 仮想サーバーの削除
サーバーを削除するには、`mactl delete server SERVER-NAME` を実行します。

```bash
$ mactl get server
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE
----             ----          ------        ---  -------  ----------       -------          ---
srv02-1          ubuntu        RUNNING       1    1024     192.168.1.200    host-bridge      8m

$ mactl delete server srv02-1
server "srv02-1" deletion requested (accepted)

$ mactl get server
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE
----             ----          ------        ---  -------  ----------       -------          ---
srv02-1          ubuntu        DELETING      1    1024     192.168.1.200    host-bridge      8m

$ mactl get server
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE
----             ----          ------        ---  -------  ----------       -------          ---
```

これで、Ubuntu 仮想マシンマネージャーで動かした仮想マシン上で、Marmotの動作が確認できました。
これができるので、Hyper-V や AWS EC2 などで、Ubuntu 24.04 を動かして、Marmotを試す事ができるであろうと思います。
