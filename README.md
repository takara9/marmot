# Marmot — プライベートマイクロクラウド

![マーモットのイメージキャラ](docs/marmot-logo.png)

Marmot（マーモット）は、テスト・学習・実験を目的としたプライベートクラウド基盤です。  
YAML ファイルで仮想サーバーやネットワークの構成を定義し、KVM / LibVirt / Open vSwitch / etcd などの Linux ネイティブテクノロジーを活用して仮想マシンを迅速に起動・管理できます。

## 特徴

- **YAML ベースの宣言的構成** — サーバー・ネットワーク・ボリュームをファイル一枚で定義
- **LVM / QCOW2 ボリューム対応** — ブートボリューム・データボリューム双方で選択可能
- **iSCSI ネットワークブロックストレージ** — LVM ボリュームを iSCSI ターゲットとして VM にアタッチ可能
- **仮想ネットワーク管理** — ホストブリッジ・OVN/Geneve オーバーレイ・VLAN をサポート
- **マルチホーム対応** — 1 台の VM に複数の仮想ネットワークを割り当て可能
- **ノードセレクター** — 複数ハイパーバイザーノードへの VM 配置制御
- **OpenAPI v3 REST API** — `marmotd` サーバーを API で完全制御
- **etcd による状態管理** — クラスター全体の状態を分散 KV ストアで保持
- **内部 DNS** — VM に対する名前解決を自動提供
- **Ubuntu 24.04 サポート**

## コンポーネント

| コンポーネント | 説明 |
|---|---|
| `marmotd` | ハイパーバイザーノード上で動作するデーモン。LibVirt / LVM / OVN(OVS) を操作して VM を管理する |
| `mactl` | CLI クライアント。YAML ファイルまたは URL を指定してサーバー・ネットワーク・ボリュームを操作する |
| `maadm` | 管理者向け補助ツール |

## クイックスタート

### 仮想サーバーの作成

```yaml
# server.yaml
name: my-server
cpu: 2
memory: 2048
os_variant: ubuntu24.04
boot_volume:
  type: qcow2
network:
  - name: "default"
```

```sh
mactl server create -f server.yaml
```

### 仮想ネットワークの作成

```yaml
# network.yaml
Metadata:
  name: my-network
Spec:
  forwardMode: bridge
```

```sh
mactl network create -f network.yaml
```

### ローカルファイルの代わりに URL を直接指定することも可能

```sh
mactl server create -f https://raw.githubusercontent.com/takara9/marmot/refs/heads/main/cmd/mactl/testdata/test-server-03-host-bridge-ip.yaml
mactl network create -f https://raw.githubusercontent.com/takara9/marmot/refs/heads/main/cmd/mactl/testdata/test-network-02-test-net-2.yaml
```

### 主な `mactl` サブコマンド

```
mactl server create    # VM を作成
mactl server delete    # VM を削除
mactl server start     # VM を起動
mactl server stop      # VM を停止
mactl server list      # VM 一覧を表示
mactl server detail    # VM 詳細を表示
mactl network create   # 仮想ネットワークを作成
mactl network delete   # 仮想ネットワークを削除
mactl network list     # 仮想ネットワーク一覧
mactl volume create    # ボリュームを作成
mactl volume list      # ボリューム一覧
mactl image list       # イメージ一覧
mactl cluster list     # クラスターノード一覧
```

## インストール

### 1. deb パッケージのインストール

[Releases](https://github.com/takara9/marmot/releases) から最新の `.deb` ファイルをダウンロードしてインストールします。

```sh
VERSION=0.13.0
curl -OL https://github.com/takara9/marmot/releases/download/v${VERSION}/marmot_v${VERSION}_amd64.deb
sudo install -m 0644 ./marmot_v${VERSION}_amd64.deb /tmp/
sudo apt install /tmp/marmot_v${VERSION}_amd64.deb
sudo rm -f /tmp/marmot_v${VERSION}_amd64.deb
```

インストール後のセットアップ手順（etcd の設定・LVM・ネットワーク・iSCSI の設定など）は [docs/HOWTO-install-marmot.md](docs/HOWTO-install-marmot.md) を参照してください。  
ハイパーバイザーノード自体の構成手順は [docs/INSTALL-SERVER.md](docs/INSTALL-SERVER.md) を参照してください。

## 主な依存技術

- [KVM / QEMU](https://www.linux-kvm.org/) — 仮想化
- [LibVirt](https://libvirt.org/) — VM ライフサイクル管理
- [OVN](https://www.ovn.org/) / [Open vSwitch](https://www.openvswitch.org/) — 仮想ネットワーク制御プレーン/データプレーン
- [etcd](https://etcd.io/) — 分散 KV ストア（クラスター状態管理）
- [LVM](https://sourceware.org/lvm2/) — 論理ボリューム管理
- [open-iscsi / targetcli](https://github.com/open-iscsi/open-iscsi) — iSCSI ネットワークブロックストレージ

## 応用例

- [設定用 Ansible 集](https://github.com/takara9/marmot-servers)
- [Kubernetes クラスターの構築](https://github.com/takara9/marmot-servers/tree/main/kubernetes)
- [Ceph ストレージシステムの構築](https://github.com/takara9/marmot-servers/tree/main/ceph)
- [メトリクス・ログ分析基盤](https://github.com/takara9/docker_and_k8s/tree/main/4-10_Observability)
- [GitHub Actions と連携した CI 環境](docs/HOWTO-CI.md)

## ライセンス

GNU General Public License v3.0 — 詳細は [LICENSE](LICENSE) を参照してください。

## 貢献

貢献を歓迎します。ガイドラインは [CONTRIBUTING.md](CONTRIBUTING.md) を参照してください。

## 連絡先

メンテナー: [takara9](https://github.com/takara9)  
ご質問・議論は [GitHub Discussions](https://github.com/takara9/marmot/discussions) へ。
