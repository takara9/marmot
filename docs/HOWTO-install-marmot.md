# marmot インストール手順

marmot を Ubuntu ハイパーバイザーノードへインストールする手順です。  
クラスターを構成する全ノードで同じ手順を実施してください。

## 前提条件

- OS: Ubuntu 24.04 (amd64)です。 22.04には対応していません。
- KVM が利用できること（`kvm-ok` で確認）
- 各ノードに SSH 公開鍵認証でログインできること
- LVM 用に使える未フォーマットのディスクが 1 本以上あること

---

## 1. debパッケージのインストール

### リリースパッケージを使う場合

GitHub Releases からパッケージをダウンロードしてインストールします。

```bash
# 2026-06-30 時点の最新バージョン
VERSION=0.25.1

# パッケージをダウンロード
curl -OL https://github.com/takara9/marmot/releases/download/v${VERSION}/marmot_v${VERSION}_amd64.deb

# インストール (依存パッケージも自動で入る)
sudo apt-get update
sudo apt install -y ./marmot_v${VERSION}_amd64.deb
sudo rm -f ./marmot_v${VERSION}_amd64.deb
```


---

## 2. 設定ファイルの確認・編集

インストール後、各ノードの設定ファイルを確認・必要に応じて編集してください。

```bash
sudo vi /etc/marmot/marmotd.json
```

主な設定項目:

| キー | 説明 | デフォルト |
|------|------|-----------|
| `node_name` | ノード名（自動でホスト名がセットされる） | `localhost` |
| `etcd_url` | etcd の URL | `http://127.0.0.1:2379` |
| `api_listen_addr` | API サーバーのリッスンアドレス | `0.0.0.0:8750` |
| `dns_listen_addr` | 内部 DNS のリッスンアドレス | `127.0.0.1:53` |
| `dns_upstream` | 上位 DNS | `8.8.8.8:53` |
| `dns_upstream_allow_cidrs` | upstream DNS への転送を許可する送信元 CIDR 一覧 | `[]` |
| `os_volume_group` | OS ボリューム用 LVM VG 名 | `vg1` |
| `data_volume_group` | データボリューム用 LVM VG 名 | `vg2` |
| `default_underlay_interface` | アンダーレイ NIC 名（Geneve/VXLAN 等で使用） | `""` |
| `tls_cert_file` | HTTPS 用証明書ファイル（未設定なら HTTP） | `""` |
| `tls_key_file` | HTTPS 用秘密鍵ファイル（未設定なら HTTP） | `""` |
| `os_images` | 起動時に自動ダウンロード・登録する OS イメージ定義 | `[]` |
| `loki_push_url` | Loki へログ送信する Push API URL（未設定なら送信なし） | `""` |
| `iscsi_server` | iSCSI ターゲットサーバーとして動作させる場合 `true` | (未設定) |

設定例:

```json
{
  "node_name": "marmot1",
  "etcd_url": "http://192.168.1.200:12379",
  "api_listen_addr": "0.0.0.0:8750",
  "dns_listen_addr": "127.0.0.1:53",
  "dns_upstream": "8.8.8.8:53",
  "dns_upstream_allow_cidrs": [
    "192.168.1.0/24"
  ],
  "default_underlay_interface": "enp4s0f0",
  "os_volume_group": "vg1",
  "data_volume_group": "vg2",
  "deletion_delay_seconds": 10,
  "tls_cert_file": "",
  "tls_key_file": ""
}
```

### marmotd.json の補足

- `dns_listen_addr` は単一ノードでローカル利用する場合 `127.0.0.1:53` のままで構いません。
- VM からホスト DNS を直接参照させる構成や、複数ノード構成では、各ノードの到達可能な IP アドレスに変更してください。
- `dns_upstream_allow_cidrs` は、外部 DNS へのフォワードを許可する送信元ネットワークを明示する用途です。空の場合は upstream 転送を制限します。
- HTTPS を使う場合は `tls_cert_file` と `tls_key_file` の両方を設定し、`api_listen_addr` は証明書のホスト名と整合するアドレスで待ち受けてください。
- `os_images` は marmotd 起動時に利用準備するイメージ一覧です。`name`、`url`、`osName`、`osVersion` を揃えて記述してください。
- `loki_push_url` は OpenTelemetry ログの送信先です。例: `http://192.168.1.9:3100/loki/api/v1/push`。
- 設定変更後は `sudo systemctl restart marmot` で再起動し、`sudo systemctl status marmot` で反映を確認します。

設定変更後はサービスを再起動します。

```bash
sudo systemctl restart marmot
sudo systemctl status marmot
```

---

## 3. LVM ボリュームグループの設定（任意）

marmot は OS ボリュームとデータボリュームに別々の VG を使います。  
新規ノードではディスクに VG を作成してください。

```bash
# ディスクを確認する
lsblk

# Physical Volume を作成する (例: /dev/vdc, /dev/vdd)
sudo pvcreate /dev/vdc
sudo pvcreate /dev/vdd

# Volume Group を作成する
sudo vgcreate vg1 /dev/vdc   # OS ボリューム用
sudo vgcreate vg2 /dev/vdd   # データボリューム用

# 確認
vgs
```

---

## 4. etcd の設定（クラスタ構成では必須、シングル構成では不要）

marmot クラスターは etcd をデータストアとして使います。  
`apt install marmot` で `etcd-server` も依存として入りますが、クラスター外部の etcd に接続する場合は次の設定は不要です。

ローカルの etcd をクラスター向けに公開する場合:

```bash
sudo systemctl edit etcd
```

以下の内容を追加します。

```ini
[Service]
Environment=ETCD_LISTEN_CLIENT_URLS="http://0.0.0.0:12379"
Environment=ETCD_ADVERTISE_CLIENT_URLS="http://0.0.0.0:12379"
```

```bash
sudo systemctl daemon-reload
sudo systemctl restart etcd
sudo systemctl status etcd
```

`/etc/marmot/marmotd.json` の `etcd_url` を etcd のアドレスに合わせて更新した後、marmot を再起動します。

---

## 5. ネットワークの設定

OVN 運用では、OVS/OVN サービス起動と libvirt 仮想ネットワーク定義を毎回同じ手順で適用します。
直接 `ovs-vsctl` を手で実行する運用は推奨しません。

### OVS/OVN サービスの確認

```bash
sudo systemctl status openvswitch-switch || true
sudo systemctl status ovn-central || true
sudo systemctl status ovn-host || true
```

### libvirt 仮想ネットワークをセットアップ

```bash
sudo -E env "PATH=$PATH" ./tools/setup-libvirt-networks.sh
virsh net-list
```

### テストや検証後のクリーンアップ

```bash
sudo -E env "PATH=$PATH" ./tools/teardown-libvirt-networks.sh
```

### 旧手順について

旧来の `ovs-vsctl add-br` など OVS 直操作の手順は移行対象です。必要な場合のみ参考として [docs/network-setup.md](network-setup.md) を参照してください。

---

## 6. open-iscsi の確認

iSCSI 機能を使う場合は `iscsid` が起動していることを確認します。

```bash
sudo systemctl status iscsid

# initiator IQN の確認 (各ノードに固有の値が入っている)
cat /etc/iscsi/initiatorname.iscsi
```

iSCSI ターゲットサーバーにするノードは `/etc/marmot/marmotd.json` に次を追加します。

```json
"iscsi_server": true
```

設定後に marmot を再起動すると、そのノードが iSCSI ターゲットサーバーとして選択されるようになります。

---

## 7. mactl クライアントの設定

`mactl` は marmot API への接続先を `~/.marmot` ファイルで管理します。

未作成の場合は、次のコマンドでデフォルトの設定ファイルを生成してから編集します。

```bash
mactl version
```

```bash
vi ~/.marmot
```

設定例:

```yaml
current: 0
endpoints:
  - https://192.168.1.200:8750
insecure-skip-tls-verify: false
```

複数のノードを列挙して `current` で切り替えることができます。

```yaml
current: 0
endpoints:
  - https://192.168.1.200:8750   # marmot1
  - https://192.168.1.201:8750   # marmot2
insecure-skip-tls-verify: false
```

### .marmot の補足

- `current` は `endpoints` の 0 始まりインデックスです。
- `insecure-skip-tls-verify: true` は自己署名証明書の検証をスキップする開発用設定です。本番環境では `false` を推奨します。
- API が HTTP 運用の場合のみ、`endpoints` を `http://` で指定してください。

---

## 8. 動作確認

```bash
# クラスターのノード一覧を確認する
mactl cluster list

# 仮想ネットワーク一覧を確認する
mactl network list

# イメージ一覧を確認する
mactl image list
```

正常にインストールされていれば、クラスターに参加しているノードが表示されます。

---

## アンインストール

```bash
sudo apt remove marmot

# 設定ファイルも削除する場合
sudo apt purge marmot
```

`purge` を実行すると `/usr/local/marmot` と `/etc/marmot` が削除されます。  
LVM ボリュームグループは削除されません。必要に応じて `vgremove` で手動削除してください。
