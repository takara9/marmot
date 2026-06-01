# marmot クラスタ構成の設定方法

## 最初のノード

- インストール先のUbuntu 24.04をインストールする。
- `scp marmot_v0.18.0_amd64.deb 192.168.1.201:/tmp` サーバーのIPアドレスを指定して、インストールパッケージを転送する。
- /tmpでインストール作業を実施する

```console
root@marmot1:/tmp# apt-get update
root@marmot1:/tmp# apt install -y ./marmot_v0.18.0_amd64.deb 
```

起動を確認

```console
root@marmot1:/tmp# systemctl status marmot |head -n 3
● marmot.service - marmot - vm cluster service
     Loaded: loaded (/usr/lib/systemd/system/marmot.service; enabled; preset: enabled)
     Active: active (running) since Mon 2026-06-01 21:26:55 UTC; 2min 51s ago
root@marmot1:/tmp# systemctl status etcd |head -n 3
● etcd.service - etcd - highly-available key value store
     Loaded: loaded (/usr/lib/systemd/system/etcd.service; enabled; preset: enabled)
     Active: active (running) since Mon 2026-06-01 21:26:05 UTC; 3min 51s ago
```

marmot と etcd を停止

```console
root@marmot1:/tmp# systemctl stop marmot
root@marmot1:/tmp# systemctl stop etcd
root@marmot1:/tmp# systemctl status etcd |head -n 1
○ etcd.service - etcd - highly-available key value store
root@marmot1:/tmp# systemctl status marmot |head -n 1
○ marmot.service - marmot - vm cluster service
```

etcdをクラスタメンバーからアクセスできるように、etcd の設定を修正
ここでは、専用プライベートネットワークなので、IPアドレスの制限を設けない様にした。

```console
vi /usr/lib/systemd/system/etcd.service
cat /usr/lib/systemd/system/etcd.service |grep Environment=DAEMON_ARGS=
Environment=DAEMON_ARGS="--listen-client-urls 'http://0.0.0.0:2379' --advertise-client-urls 'http://0.0.0.0:2379'"
```

設定が反映されるようにリロードして、etcdを起動する

```
root@marmot1:/tmp# systemctl daemon-reload
root@marmot1:/tmp# systemctl start etcd
root@marmot1:/tmp# systemctl status etcd |head -n 3
● etcd.service - etcd - highly-available key value store
     Loaded: loaded (/usr/lib/systemd/system/etcd.service; enabled; preset: enabled)
     Active: active (running) since Mon 2026-06-01 21:42:03 UTC; 14s ago
```

marmot の設定を編集する
最小限は、以下３つのエントリを環境にあわせて修正する
- etcd_url
- dns_listen_addr
- dns_upstream

```console
root@marmot1:/tmp# vi /etc/marmot/marmotd.json 
root@marmot1:/tmp# cat /etc/marmot/marmotd.json |head -n 7
{
  "node_name": "marmot1",
  "etcd_url": "http://172.16.0.201:2379",
  "api_listen_addr": "0.0.0.0:8750",
  "dns_listen_addr": "192.168.1.201:53",
  "dns_upstream": "192.168.1.9:53",
  "dns_upstream_allow_cidrs": [
```

DNSの宛先を自ホストのパブリックIPへ変更する

```console
root@marmot1:/tmp# cat /etc/resolv.conf 
# generate by marmotd
nameserver 192.168.1.201
options edns0 trust-ad
search host-bridge
```

hostsの一行目に、自ホスト名を追加する

```console
root@marmot1:/tmp# cat /etc/hosts |head -n 1 
127.0.0.1 localhost marmot1
```

marmotを起動する

```console
root@marmot1:/tmp# systemctl daemon-reload
root@marmot1:/tmp# systemctl start marmot
root@marmot1:/tmp# systemctl status marmot |head -n 3
● marmot.service - marmot - vm cluster service
     Loaded: loaded (/usr/lib/systemd/system/marmot.service; enabled; preset: enabled)
     Active: active (running) since Mon 2026-06-01 21:49:58 UTC; 12s ago
```

mactl から起動を確認する。 versionが表示されたらOKと見なせる

```console
root@marmot1:/tmp# mactl version
{"time":"2026-06-01T21:50:50.715813138Z","level":"INFO","source":{"function":"github.com/takara9/marmot/pkg/config.EnsureMarmotConfig","file":"/home/ubuntu/marmot/pkg/config/endpoint-config.go","line":28},"msg":"設定ファイルを作成しました","path":"/root/.marmot","from":"/etc/marmot/.marmot.example"}
Getting server Information...
Host and Port: localhost:8750
API Path: /api/v1
Scheme: http
Server version = 0.18.0

Client version = 0.18.0
```




### ２台目のメンバーを追加する

hostsの一行目に、自ホスト名を追加する

```console
root@marmot2:/tmp# cat /etc/hosts |head -n 1 
127.0.0.1 localhost marmot2
```

インストールパッケージを/tmpに転送しておく。

```console
root@marmot2:/tmp# apt-get update && apt install -y ./marmot_v0.18.0_amd64.deb
```

marmotとetcdを停止する。
etcdは起動しないようにする。

```console
systemctl stop marmot
systemctl stop etcd
systemctl disable etcd
systemctl is-enabled etcd
```

marmotd.jsonを編集して、マスターノードのetcdを参照するように変更する

```console
root@marmot2:/tmp# vi /etc/marmot/marmotd.json 
root@marmot2:/tmp# cat /etc/marmot/marmotd.json  | head -n 6
{
  "node_name": "marmot2",
  "etcd_url": "http://172.16.0.201:2379",
  "api_listen_addr": "0.0.0.0:8750",
  "dns_listen_addr": "192.168.1.202:53",
  "dns_upstream": "192.168.1.9:53",
root@marmot2:/tmp# 
```

marmotd を起動する

```console
root@marmot2:/tmp# systemctl daemon-reload
root@marmot2:/tmp# systemctl start marmot
root@marmot2:/tmp# systemctl status marmot |head -n 3
● marmot.service - marmot - vm cluster service
     Loaded: loaded (/usr/lib/systemd/system/marmot.service; enabled; preset: enabled)
     Active: active (running) since Mon 2026-06-01 22:02:11 UTC; 10s ago
```

DNSの宛先を自ホストのパブリックIPへ変更する

```console
root@marmot1:/tmp# cat /etc/resolv.conf 
# generate by marmotd
nameserver 192.168.1.202
options edns0 trust-ad
search host-bridge
```

### marmotクラスタに参加したことを確認

２台目が表示されていること。リストに載ってこなければ、作業を再確認する

```console
root@marmot1:/tmp# mactl marmot cluster
NODE             HOSTID     IP              CAP_CPU CAP_MEM(MB)  TOTAL RUNNING STOPPED     VCPU  MEM(MB)    VNETS UPDATED
marmot1          2ec811c2   172.16.0.201          8      15991      0       0       0        0        0        4 2026-06-01 22:03:28
marmot2          e1fbf903   172.16.0.202          8      15991      0       0       0        0        0        4 2026-06-01 22:03:32
```


### 3台目のメンバー追加

同じ要領で、３台目を追加して、`mactl marmot cluster`で表示されたら成功と見なせる。


## イメージの同期

```console
ubuntu@ws1:~$ mactl get img
NAME            NODE-NAME  STATUS     ROLE   LV   QCOW2  AGE
----            ---------  ------     ----   --   -----  ---
ubuntu22.04     marmot1    AVAILABLE  master  no   yes    48m
ubuntu24.04     marmot1    AVAILABLE  master  no   yes    48m
ubuntu@ws1:~$ mactl del img ubuntu22.04
image "ubuntu22.04" deleted successfully
ubuntu@ws1:~$ mactl del img ubuntu24.04
image "ubuntu24.04" deleted successfully
ubuntu@ws1:~$ mactl get img
NAME            NODE-NAME  STATUS     ROLE   LV   QCOW2  AGE
----            ---------  ------     ----   --   -----  ---
ubuntu@ws1:~$ 
```

ヘッドノードを再起動する

```console
root@marmot1:/tmp# systemctl restart marmot
```

イメージの作成とレプリカが自動的に開始される
```console
ubuntu@ws1:~$ mactl get img
NAME            NODE-NAME  STATUS     ROLE   LV   QCOW2  AGE
----            ---------  ------     ----   --   -----  ---
ubuntu22.04     marmot1    CREATING   master  no   no     30s
ubuntu24.04     marmot1    CREATING   master  no   no     30s
ubuntu22.04     marmot2    WAITING    replica  no   yes    24s
ubuntu22.04     marmot3    WAITING    replica  no   yes    24s
ubuntu24.04     marmot2    WAITING    replica  no   yes    24s
ubuntu24.04     marmot3    WAITING    replica  no   yes    24s
```

各ノードでimageが利用可能になれば、完了

```console
ubuntu@ws1:~$ mactl get img
NAME            NODE-NAME  STATUS     ROLE   LV   QCOW2  AGE
----            ---------  ------     ----   --   -----  ---
ubuntu22.04     marmot1    AVAILABLE  master  no   yes    1m
ubuntu24.04     marmot1    AVAILABLE  master  no   yes    1m
ubuntu22.04     marmot2    AVAILABLE  replica  no   yes    1m
ubuntu22.04     marmot3    AVAILABLE  replica  no   yes    1m
ubuntu24.04     marmot2    AVAILABLE  replica  no   yes    1m
ubuntu24.04     marmot3    AVAILABLE  replica  no   yes    1m
```

以上