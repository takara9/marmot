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
- **ゲストOS Ubuntu Linux 22.04/24.04, Alpine Linux 3.23 サポート**
- **ホストOS Ubuntu 24.04 サポート**

## コンポーネント

| コンポーネント | 説明 |
|---|---|
| `marmotd` | ハイパーバイザーノード上で動作するデーモン。LibVirt / LVM / OVN(OVS) を操作して VM を管理する |
| `mactl` | CLI クライアント。YAML ファイルまたは URL を指定してサーバー・ネットワーク・ボリュームを操作する |
| `maadm` | 管理者向け補助ツール |

## クイックスタート

### 仮想サーバーの作成

仮想サーバーを起動するためのマニフェスト

```yaml
apiVersion: v1
kind: Server
metadata:
    name: server-20
    comment: marmotホストが繋がるネットワークに接続する仮想サーバー
spec:
    cpu: 1
    memory: 1024
    osVariant: ubuntu24.04
    auth: # 利用者の公開鍵に変更してください。
        url: https://github.com/takara9.keys
    networkInterface:
        - networkname: host-bridge  # marmot のサーバーが接続されるネットワーク
          address: 192.168.1.20     # IPアドレスを手動設定（IPアドレスの重複使用に注意)
          netmasklen: 24            # ネットマスク
          routes:                   # デフォルトGW ルーターのアドレスを指定
            - to: default
              via: 192.168.1.1
          nameservers:              # DNSサーバー
            addresses:
                - 192.168.1.9       # ローカル環境のDNSサーバー
            search:                 # ドメイン名を省略可能なドメインをセット
                - labo.local
```

次のKubernetesライクなコマンドで、仮想サーバーを起動

```console
$ mactl create -f server-20.yaml 
リソースの作成要求が受け入れられました。ID: 9451a

$ mactl get srv
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE
----             ----          ------        ---  -------  ----------       -------          ---
server-20        marmot3       RUNNING       1    1024     192.168.1.20     host-bridge      5s

$ ssh 192.168.1.20
ubuntu@server-20:~$ 
```

## インストール

### 必要最小条件
- Ubuntu Linux 24.06 がインストールされていること。
- ルート`/`ファイルシステムが、3G程度空いていること。 marmotが依存モジュール 1.2GBほどが、インストールされます。
- `/var/lib/marmot` が、100GB程度の空きがあること。（OSイメージ取得で約2GB、１台の仮想マシン起動で16GBを消費します。


### deb パッケージのインストール

[Releases](https://github.com/takara9/marmot/releases) から最新の `.deb` ファイルをダウンロードしてインストールします。

```console
cd /tmp
apt-get update
VERSION=0.23.0
curl -OL https://github.com/takara9/marmot/releases/download/v${VERSION}/marmot_v${VERSION}_amd64.deb
sudo apt install -y ./marmot_v${VERSION}_amd64.deb
```
インストール完了後に、`/etc/marmot/marmotd.json`を編集して、`systemctl restart mamort` を実行します。
シングル構成時は、以下の２箇所に注意してください。

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


```console
sudo rm -f ./marmot_v${VERSION}_amd64.deb
```

## 主な利用技術

- [KVM / QEMU](https://www.linux-kvm.org/) — 仮想化
- [LibVirt](https://libvirt.org/) — VM ライフサイクル管理
- [OVN](https://www.ovn.org/) / [Open vSwitch](https://www.openvswitch.org/) — 仮想ネットワーク制御プレーン/データプレーン
- [etcd](https://etcd.io/) — 分散 KV ストア（クラスター状態管理）
- [LVM](https://sourceware.org/lvm2/) — 論理ボリューム管理
- [open-iscsi / targetcli](https://github.com/open-iscsi/open-iscsi) — iSCSI ネットワークブロックストレージ

## 応用例

- [marmotマニフェスト集](https://github.com/takara9/marmot-manifests)

## チュートリアル
準備中

## リファレンスマニュアル
準備中

## ライセンス

GNU General Public License v3.0 — 詳細は [LICENSE](LICENSE) を参照してください。

## 貢献

貢献を歓迎します。ガイドラインは [CONTRIBUTING.md](CONTRIBUTING.md) を参照してください。

## About the Author

Marmotは、30年以上にわたりエンタープライズシステム、仮想化基盤、クラウド技術に携わってきたインフラエンジニアによって開発されています。

長年にわたり、大規模システムの構築・運用を経験する中で、クラウド基盤は便利になる一方で、導入や運用が複雑化し、多くの利用者にとって扱いにくいものになっていると感じてきました。

Marmotは、その課題に対する一つの答えとして生まれました。

「プライベートクラウドをもっとシンプルに。」

この理念のもと、Kubernetesの宣言型運用の良さを取り入れながら、より少ない学習コストで利用できるクラウド基盤を目指しています。

Marmotは、複雑な仕組みを増やすのではなく、本当に必要な機能に集中し、学習・検証・実験から実運用まで幅広く活用できるプラットフォームとして進化を続けています。


## 連絡先

メンテナー: [takara9](https://github.com/takara9)  
ご質問・議論は [GitHub Discussions](https://github.com/takara9/marmot/discussions) へ。
