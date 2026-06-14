# Marmot — プライベートマイクロクラウド

Marmot（マーモット）は、テスト・学習・実験を目的としたプライベートクラウド基盤です。 KubernetesライクなコマンドとYAMLで記述したAPIファイルで、仮想サーバーやネットワークの構成を定義し、仮想マシンを迅速に起動・管理できます。

![マーモットのイメージキャラ](docs/marmot-logo-quarter.png)


## 特徴

- **YAML ベースの宣言的構成** — サーバー・ネットワーク・ボリュームをファイル一枚で定義
- **LVM / QCOW2 ボリューム対応** — データボリュームで選択可能
- **iSCSI ネットワークブロックストレージ** — LVM ボリュームを iSCSI ターゲットとして VM にアタッチ可能
- **仮想ネットワーク管理** — ホストブリッジ・OVN/Geneve オーバーレイ・VLAN をサポート
- **マルチホーム対応** — 1 台の VM に複数の仮想ネットワークを割り当て可能
- **ノードセレクター** — 複数ハイパーバイザーノードへの VM 配置制御
- **OpenAPI v3 REST API** — `marmotd` サーバーを API で完全制御
- **etcd による状態管理** — クラスター全体の状態を分散 KV ストアで保持
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

### 1. deb パッケージのインストール

[Releases](https://github.com/takara9/marmot/releases) から最新の `.deb` ファイルをダウンロードしてインストールします。

```console
VERSION=0.23.0
curl -OL https://github.com/takara9/marmot/releases/download/v${VERSION}/marmot_v${VERSION}_amd64.deb
sudo install -m 0644 ./marmot_v${VERSION}_amd64.deb /tmp/
sudo apt install /tmp/marmot_v${VERSION}_amd64.deb
sudo rm -f /tmp/marmot_v${VERSION}_amd64.deb
```

## 主な依存技術

- [KVM / QEMU](https://www.linux-kvm.org/) — 仮想化
- [LibVirt](https://libvirt.org/) — VM ライフサイクル管理
- [OVN](https://www.ovn.org/) / [Open vSwitch](https://www.openvswitch.org/) — 仮想ネットワーク制御プレーン/データプレーン
- [etcd](https://etcd.io/) — 分散 KV ストア（クラスター状態管理）
- [LVM](https://sourceware.org/lvm2/) — 論理ボリューム管理
- [open-iscsi / targetcli](https://github.com/open-iscsi/open-iscsi) — iSCSI ネットワークブロックストレージ

## 応用例

- [marmotマニフェスト集](https://github.com/takara9/marmot-manifests)

## ライセンス

GNU General Public License v3.0 — 詳細は [LICENSE](LICENSE) を参照してください。

## 貢献

貢献を歓迎します。ガイドラインは [CONTRIBUTING.md](CONTRIBUTING.md) を参照してください。

## 連絡先

メンテナー: [takara9](https://github.com/takara9)  
ご質問・議論は [GitHub Discussions](https://github.com/takara9/marmot/discussions) へ。
