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

GitHub Releases からパッケージをダウンロードしてインストールします。

```bash
sudo apt-get update
sudo apt install curl
MARMOT_VER=0.15.0
# パッケージをダウンロード
curl -OL https://github.com/takara9/marmot/releases/download/v${MARMOT_VER}/marmot_v${MARMOT_VER}_amd64.deb
# インストール (依存パッケージも自動で入る)
sudo apt install ./marmot_v${MARMOT_VER}_amd64.deb
```

[!NOTE] 以下のエラーが出ますが、問題ありませんので、続行してください。
```
No VM guests are running outdated hypervisor (qemu) binaries on this host.
N: Download is performed unsandboxed as root as file '/home/ubuntu/marmot_v0.14.1_amd64.deb' couldn't be accessed by user '_apt'. - pkgAcquire::Run (13: Permission denied)
```


インストール後の動作確認

インストールしたバージョンを確認するコマンドを実行します。
クライアントとサーバーのバージョンが表示されると、正常にインストールできたことになります。
```
$ mactl version
{"time":"2026-05-16T08:12:53.479163594Z","level":"INFO","source":{"function":"github.com/takara9/marmot/pkg/config.EnsureMarmotConfig","file":"/home/ubuntu/release-marmot/pkg/config/endpoint-config.go","line":28},"msg":"設定ファイルを作成しました","path":"/home/ubuntu/.marmot","from":"/etc/marmot/.marmot.example"}
Getting server Information...
Host and Port: localhost:8750
API Path: /api/v1
Scheme: http
Server version = 0.15.0

Client version = 0.15.0
```

次のコマンドで、現在のリソースの状況を表示してくれます。

```
$ mactl marmot status
ホスト情報:
  ノード名:       marmot-fre
  hostId:         68ad60ac
  IPアドレス:     172.16.0.204

資源搭載量:
  vCPU数:         8
  メモリ搭載量:   15991 MB
  ディスク本数:   0
  ディスク容量:   0 GB
  ネットワークIF: [enp1s0 enp2s0 virbr0]

割当数:
  VM数（合計）:       0
  VM数（稼働中）:     0
  VM数（停止中）:     0
  vCPU割当数:         0 vCPU（稼働中のみ）
  メモリ割当量:       0 MB（稼働中のみ）
  仮想ネットワーク数: 1

最終更新: 2026-05-16 08:13:26
```

## 2. OSイメージのダウンロード

次のコマンドで、Ubuntu 24.04 のクラウドイメージをダウンロードして、
Marmotのマシンイメージを作成します。

```
cat <<EOF | mactl apply -f -
apiVersion: v1
kind: Image
metadata:
    name: ubuntu24.04
spec:
    sourceUrl: https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img
EOF
```

数分後に、次のように STATUS が AVAILABLEになったら、このマシンイメージを利用して、
仮想サーバーが起動できるようになります。

```
$ mactl get img
NAME            NODE-NAME  STATUS     ROLE   LV   QCOW2  AGE
----            ---------  ------     ----   --   -----  ---
ubuntu24.04     marmot-fre  AVAILABLE  master  no   yes    1m
```


## 3. 稼働サーバーの起動

次のマニフェストを適用すると、コンソールにログインして、操作できる仮想サーバーが作成できます。

```
cat <<EOF |mactl apply -f -
apiVersion: v1
kind: Server
metadata:
    name: server-01
    comment: 何もしないミニマルな設定
spec:
    cpu: 1
    memory: 1024
    osVariant: ubuntu24.04
    networkInterface:
        - networkname: default
EOF
```

稼働中の仮想サーバーをリストする。

```
$ sudo virsh list
 Id   Name              State
---------------------------------
 3    server-01-47347   running
```

仮想サーバーにコンソールからログインする。

```
$ sudo virsh console server-01-47347
Connected to domain 'server-01-47347'
Escape character is ^] (Ctrl + ])

server-01 login: root
Password: 
Welcome to Ubuntu 24.04.4 LTS (GNU/Linux 6.8.0-106-generic x86_64)
```





## 2. ネットワークの設定

インストール直後は、次の様に、defaultネットワークだけです。
このデフォルトネットワークに接続して

```
$ mactl get net
NAME            NODE-NAME  BRIDGE-NAME   IP-NET             STATUS   AGE
----            ---------  -----------   ------             ------   ---
default         marmot-fre  virbr0        -                   ACTIVE   1m
```




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
| `default_underlay_interface` | アンダーレイ NIC 名（VXLAN 等で使用） | `""` |
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

libvirt 仮想ネットワークと OVS ブリッジの設定が必要です。  
詳細は [docs/HOWTO-setup-vm-runner.md](HOWTO-setup-vm-runner.md) および [docs/network-setup.md](network-setup.md) を参照してください。

### OVS ブリッジの作成

```bash
sudo ovs-vsctl add-br ovsbr0
sudo ovs-vsctl add-port ovsbr0 <物理NIC名>   # 例: enp4s0f0
sudo ovs-vsctl show
```

### libvirt 仮想ネットワークの定義

```bash
virsh net-define tools/ovs-network.xml
virsh net-start ovs-network
virsh net-autostart ovs-network

virsh net-define tools/host-bridge.xml
virsh net-start host-bridge
virsh net-autostart host-bridge

virsh net-list
```

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
