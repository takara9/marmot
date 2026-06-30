# marmot シングル構成のインストール手順

marmot を Ubuntu ハイパーバイザーノードへインストールする手順です。  


## 前提条件

- OS: Ubuntu 24.04 (amd64)です。 22.04には対応していません。
- KVM が利用できること（`kvm-ok` で確認）
- SSH 公開鍵認証でログインできること

---

## 1. debパッケージのインストール

GitHub Releases からパッケージを/tmpにダウンロードしてインストールします。

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

## アンインストール

```bash
sudo apt remove marmot

# 設定ファイルも削除する場合
sudo apt purge marmot
```

アンインストール時の注意: 
- `purge` を実行すると `/usr/local/marmot` と `/etc/marmot` が削除されます。  
- LVM ボリュームグループは削除されません。必要に応じて `vgremove` で手動削除してください。
- /etc/netplan/下のファイルは、復元されません。 .bakファイルが作成されているので、これを利用して設定を復旧してください。
