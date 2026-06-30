# Marmot — プライベートクラウド基盤

Marmot（マーモット）は、シンプルなYAMLで仮想サーバー、ネットワーク、ストレージを管理できるプライベートクラウド基盤です。Kubernetesライクな宣言型APIを採用し、学習・検証環境から実運用まで、迅速かつ再現性の高いインフラ構築を実現します。

![マーモットのイメージキャラ](docs/marmot-logo-quarter.png)

## コンセプト

利用者は、Kubernetesのマニフェストを書く感覚で仮想マシンを管理できます。しかし、Kubernetesのような複雑な概念や高い学習コストはありません。シンプルで理解しやすいYAMLを記述するだけで、仮想サーバー、ネットワーク、ストレージを宣言的に管理できます。[Marmotマニフェスト集](https://github.com/takara9/marmot-manifests)を参考にすれば、Linuxをインストールした1台のマシンから始めて、プライベートネットワークの構築、仮想ストレージの確保、仮想サーバーの展開までを数分で実現できます。複雑な設定作業に悩まされることなく、学習、検証、実験に集中できます。


```yaml
apiVersion:
kind:
spec:
```

Marmotは、複雑化し続けるインフラ運用をシンプルにするためのプライベートクラウド基盤です。パブリッククラウドのコスト増大、高額なVMwareライセンス、GUI依存の運用、そしてOpenStackの重厚なアーキテクチャに代わり、シンプルなYAMLによるIaCを中心とした運用モデルを提供します。Ansibleによる自動構成管理やコンテナとの連携を標準化し、小規模なチームでも効率的にクラウド基盤を構築・運用できる世界を目指します。

Marmotは、プライベートクラウド運用をよりシンプルにし、Infrastructure as Codeを誰もが実践できる環境を目指しています。


## 技術的特徴
**marmot**に実装した魅力的な特徴を以下に列挙します。この複合的で統合された環境を**marmot**をインストールすることで手に入れることができます。

- **YAML ベースの宣言的構成** — サーバー・ネットワーク・ボリュームをファイル一枚で定義
- **OpenAPI v3 REST API** — `marmotd` サーバーを API で完全制御
* **リソースコントローラーによる自動化** — 利用者は「どうなっていてほしいか」をYAMLで記述するだけ。Marmotが仮想マシンやネットワークの作成・変更を自動的に実行します。
- **Cloud Image による仮想マシン** — UbuntuやAlmaLinuxなどのCloud Imageを利用して、OSインストールの手間なく仮想マシンを即座に作成できます。
- **iSCSI ネットワークブロックストレージ** — LVM ボリュームを iSCSI ターゲットとして VM にアタッチ可能
- **ローカルブロックストレージ** — LVM / QCOW2 を利用した高速にアクセスできるストレージをVM にアタッチ可能
- **仮想ネットワーク管理** — ホストブリッジ・OVN/Geneve オーバーレイ・VLAN をサポート
- **マルチホーム対応** — 1 台の VM に複数の仮想ネットワークを割り当て可能
- **ノードセレクター** — marmotクラスタ環境では、複数のMarmotノードから指定ノードへVMを 配置制御可能
- **etcd による状態管理** — Kubernetesで実績のあるetcdを利用し、クラスター全体の状態を安全かつ一貫性のある形で管理します。
- **内部 DNS** — VM に対する名前解決を自動提供
- **OpenVPN ゲートウェイ** — VPN Gateway リソースで、PCとVM用プライベートネットワーク間を暗号化接続
- **Application Load Balancer** — HAProxy ベースの L7 ロードバランサーを VM として自動デプロイ
- **Network Load Balancer** — iptables ベースの L4 ロードバランサーで仮想ネットワーク内トラフィックを分散
- **OpenTelemetry メトリクス** — Prometheus エクスポーターによる可観測性を標準提供, Loki へのログ転送も標準装備
- **RBAC によるアクセス制御** — Administrator / Compute-Operator / Viewer などのロールベースで API 操作を制限
- **ゲストOS Ubuntu Linux 22.04/24.04, Alpine Linux 3.23 サポート**
- **ホストOS Ubuntu 24.04 サポート**



## クイックスタート
インストールして利用する前に、このパートで使用体験を感じてみてください。

### ログイン

`marmotd` への接続は認証が必要です。初期管理者アカウント（`admin`）でログインします。
インストール直後のパスワードは、`passw0rd`ですので、下記の手順で変更してください。

```console
$ mactl login admin
Password: ＜パスワードを入力＞
Successfully logged in as admin
⚠️  You must change your password before using other commands.
   Run: mactl passwd
```

初回ログイン時はパスワード変更を求められます。`mactl passwd` で変更してください。

```console
$ mactl passwd
Current Password: ＜現在のパスワードを入力＞
New Password:     ＜新しいパスワードを入力（英数字 8 文字以上）＞
```

ログイン中のユーザー情報は `mactl whoami` で確認できます。

```console
$ mactl whoami
User:  admin
Roles: [Administrator]
```

セッションを終了するときは `mactl logout` を実行します。

```console
$ mactl logout
Successfully logged out
```

### 仮想サーバーの作成

sshの鍵ペアを準備して、公開鍵を表示します。
```console
$ ssh-keygen -t ed25519 -f ./vmkey -C "For marmot VMs" -N ""
$ cat ./vmkey.pub 
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGAAL3eo+VgR6pj9eGuz62rBp/wbs4dSp3XljqBBymnW For marmot VMs
```
仮想サーバーを起動するため、上記で作成した公開鍵をspec.auth.publicKeyに貼り付けたマニフェストを準備します。

```yaml
apiVersion: v1
kind: Server
metadata:
    name: server-20
    comment: marmotホストが繋がるネットワークに接続する仮想サーバー
spec:
    cpu: 1
    memory: 1024
    mmImage: ubuntu24.04
    auth:
        publicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGAAL3eo+VgR6pj9eGuz62rBp/wbs4dSp3XljqBBymnW For marmot VMs"
        users:
         - ubuntu       
    networkInterface:
        - networkname: host-bridge  # marmot のサーバーが接続されるネットワーク
          address: 192.168.1.20     # IPアドレスを手動設定（IPアドレスの重複使用に注意)
          netmasklen: 24            # ネットマスク
          routes:                   # デフォルトGW ルーターのアドレスを指定
            - to: default
              via: 192.168.1.1
          nameservers:              # 仮想サーバーが参照するDNSサーバー
            addresses:
                - 8.8.8.8
            search:                 # ドメイン名を省略可能なドメインをセット
                - labo.local
```

### 仮想サーバーの作成

次のKubernetesライクなコマンドで、仮想サーバーを起動できます。
環境により、しばらく時間が必要なことがあります。

```console
# サーバーの作成
$ mactl create -f server-20.yaml 

# 起動の確認
$ mactl get server
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE
----             ----          ------        ---  -------  ----------       -------          ---
server-20        marmot3       RUNNING       1    1024     192.168.1.20     host-bridge      5s

# sshでのログイン
$ ssh ubuntu@192.168.1.20 -i vmkey
ubuntu@server-20:~$ 
ubuntu@server-20:~$ exit

# サーバーの削除
$ mactl delete server server-20
```


## インストール

### 必要最小条件
- Ubuntu Linux 24.04 がインストールされていること。
- ルート`/`ファイルシステムが、3G程度空いていること。 marmotが依存モジュール 1.2GBほどが、インストールされます。
- `/var/lib/marmot` が、30GB程度の空きがあること。（OSイメージ取得で約2GB、１台の仮想マシン起動で16GBを消費します。


### deb パッケージのインストール

[Releases](https://github.com/takara9/marmot/releases) から最新の `.deb` ファイルをダウンロードしてインストールします。

```console
cd /tmp
apt-get update
VERSION=0.25.1
curl -OL https://github.com/takara9/marmot/releases/download/v${VERSION}/marmot_v${VERSION}_amd64.deb
sudo apt install -y ./marmot_v${VERSION}_amd64.deb
```

### marmotdの設定ファイルの編集

marmotd は、`/etc/marmot/matmotd.json` を読み込んで起動します。
デフォルト設定では、動作しないケースがありますので、インストール完了後に、`/etc/marmot/marmotd.json`を編集して、`systemctl restart marmot` を実行します。シングル構成時は、以下の２箇所に注意してください。


```json
# cat marmotd.json 
{
  "node_name": "marmot0",
  "etcd_url": "http://127.0.0.1:2379",
  "api_listen_addr": "0.0.0.0:8750",
  "dns_listen_addr": "127.0.0.1:53", # 仮想マシンのDNS名を参照する時に、hostのIPアドレスに変更
  "dns_upstream": "8.8.8.8:53",      # 内部DNSで解決できない時の上位 DNSサーバー
  "dns_upstream_allow_cidrs": [
    "192.168.1.0/24"
  ],
  "default_underlay_interface": "",
以下省略
```

インストールが完了したら、ダウンロードしたファイルは消しておくのがお勧めです。
```console
sudo rm -f ./marmot_v${VERSION}_amd64.deb
```

### クライアント側の設定ファイル

mactl コマンドは、ホームディレクトリの`.marmot`ファイルから、接続先とプロトコルを選択して marmotd と連携します。
`mactl version` を実行することで、初期設定の入った`.marmot`が作成されるので、編集して利用してください。

```yaml
current: 0
endpoints:
  - http://localhost:8750        # marmotd が、TLS暗号化が有効なっていれば https に変更する
insecure-skip-tls-verify: false  # サーバーの証明書が、自己証明書（オレオレ証明書）の場合は、tureにする。
```

実行結果に、Serverのバージョンが表示されれば、サーバーと正しく通信できていることになります。

```console
$ mactl vesion
Server version = 0.25.0
Client version = 0.25.0
```

## チュートリアル
はじめて **marmot** を利用する方は、チュートリアルから始めるのがお勧めです。
基本操作を把握できる 30分コースと、じっくり楽しめる2時間コースを用意しています。

- [Marmot Tutorial](https://github.com/takara9/marmot-manifests/blob/main/TUTORIAL.md)


## 活用例
**marmot** の利用法を確認できるマニフェスト集もあります。このマニフェスト集から、順番を選んでチュートリアルを作成しました。

- [marmotマニフェスト集](https://github.com/takara9/marmot-manifests)


## リファレンスマニュアル
コマンドとサブコマンド、オプションについての知りたい時は、リファレンスマニュアルをお勧めします。

- [Command Reference](https://github.com/takara9/marmot-manifests/blob/main/COMMAND_REFERENCE.md)


## コンポーネント

| コンポーネント | 説明 |
|---|---|
| `marmotd` | ハイパーバイザーノード上で動作するデーモン。LibVirt / LVM / OVN(OVS) を操作して VM を管理する |
| `mactl` | CLI クライアント。YAML ファイルまたは URL を指定してサーバー・ネットワーク・ボリュームを操作する |
| `maadm` | 管理者向け補助ツール（改修中のため使用は非推奨） |


## 主な利用技術
**marmot**は、複数のOSS技術を利用して構成されています。現在、使用しているOSS技術を以下に列挙します。

- [KVM / QEMU](https://www.linux-kvm.org/) — 仮想化
- [LibVirt](https://libvirt.org/) — VM ライフサイクル管理
- [OVN](https://www.ovn.org/) / [Open vSwitch](https://www.openvswitch.org/) — 仮想ネットワーク制御プレーン/データプレーン
- [etcd](https://etcd.io/) — 分散 KV ストア（クラスター状態管理）
- [LVM](https://sourceware.org/lvm2/) — 論理ボリューム管理
- [open-iscsi / targetcli](https://github.com/open-iscsi/open-iscsi) — iSCSI ネットワークブロックストレージ
- [OpenTelemetry](https://opentelemetry.io/ja/) — 可観測性のためのメトリックス公開、ログ送信のために使用
- [Geneve protocl](https://www.redhat.com/ja/blog/what-geneve) — marmotdをクラスタ構成に使用するオーバーレイ・ネットワークに使用します。
- [OpenVPN](https://openvpn.net/) — marmotd が管理するプライベートネットワークにセキュアに接続するために使用します。 
- [Go言語](https://go.dev/) — **marmot**は、Go言語で書かれています。
- [Ansible](https://docs.ansible.com/) — **mactl**コマンドは、Ansibleと連携して、起動した仮想サーバーをセットアップします。

## ライセンス

GNU General Public License v3.0 — 詳細は [LICENSE](LICENSE) を参照してください。

## 貢献

貢献を歓迎します。ガイドラインは [CONTRIBUTING.md](CONTRIBUTING.md) を参照してください。

## 開発者の経歴について

Marmotは、30年以上にわたりエンタープライズシステム、仮想化基盤、クラウド技術に携わってきたインフラエンジニアによって開発されています。

長年にわたり、大規模システムの構築・運用を経験した後、クラウド基盤を利用した開発とテクニカルセールスに従事する中で、Kuberntesとコンテナは画期的な技術革新である反面、導入や運用が複雑化し、多くの利用者にとって扱いにくいものになっていると感じてきました。

Marmotは、その課題に対する一つの答えとして生まれました。

「プライベートクラウドをもっとシンプルに。」

この理念のもと、Kubernetesの宣言型運用の良さを取り入れながら、より少ない学習コストで利用できるクラウド基盤を目指しています。

Marmotは、複雑な仕組みを増やすのではなく、本当に必要な機能に集中し、学習・検証・実験から実運用まで幅広く活用できるプラットフォームとして進化を続けています。


## 連絡先

メンテナー: [takara9](https://github.com/takara9)  
ご質問・議論は [GitHub Discussions](https://github.com/takara9/marmot/discussions) へ。
