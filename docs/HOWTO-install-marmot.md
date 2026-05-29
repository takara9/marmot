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
# パッケージをダウンロード
curl -OL https://github.com/takara9/marmot/releases/download/v0.13.0/marmot_v0.13.0_amd64.deb

# インストール (依存パッケージも自動で入る)
sudo install -m 0644 ./marmot_v0.13.0_amd64.deb /tmp/
sudo apt install /tmp/marmot_v0.13.0_amd64.deb
sudo rm -f /tmp/marmot_v0.13.0_amd64.deb
```

### ソースからビルドしてインストールする場合

```bash
# ビルド
make all

# debパッケージ生成
bash tools/build-deb.sh

# インストール
sudo install -m 0644 ./dist/marmot_v0.13.0_amd64.deb /tmp/
sudo apt install /tmp/marmot_v0.13.0_amd64.deb
sudo rm -f /tmp/marmot_v0.13.0_amd64.deb
```

`dpkg postinst` スクリプトにより、以下が自動で行われます。

- `/etc/marmot/marmotd.json` の `node_name` をホスト名に書き換え
- `libvirtd`, `iscsid`, `marmot` サービスの有効化・起動

### 複数ノードへ一括デプロイする場合

```bash
# tools/deploy-deb.sh 内の HOSTS 配列を環境に合わせて編集してから実行
bash tools/deploy-deb.sh
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
| `os_volume_group` | OS ボリューム用 LVM VG 名 | `vg1` |
| `data_volume_group` | データボリューム用 LVM VG 名 | `vg2` |
| `default_underlay_interface` | アンダーレイ NIC 名（Geneve/VXLAN 等で使用） | `""` |
| `iscsi_server` | iSCSI ターゲットサーバーとして動作させる場合 `true` | (未設定) |

設定例:

```json
{
  "node_name": "marmot1",
  "etcd_url": "http://192.168.1.200:12379",
  "api_listen_addr": "0.0.0.0:8750",
  "dns_listen_addr": "127.0.0.1:53",
  "dns_upstream": "8.8.8.8:53",
  "default_underlay_interface": "enp4s0f0",
  "os_volume_group": "vg1",
  "data_volume_group": "vg2",
  "deletion_delay_seconds": 10
}
```

設定変更後はサービスを再起動します。

```bash
sudo systemctl restart marmot
sudo systemctl status marmot
```

---

## 3. LVM ボリュームグループの設定

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

## 4. etcd の設定

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

```bash
cp /etc/marmot/.marmot.example ~/.marmot
vi ~/.marmot
```

設定例:

```yaml
current: 0
endpoints:
  - http://192.168.1.200:8750
```

複数のノードを列挙して `current` で切り替えることができます。

```yaml
current: 0
endpoints:
  - http://192.168.1.200:8750   # marmot1
  - http://192.168.1.201:8750   # marmot2
```

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
