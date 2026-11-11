# marmot cluster構成のインストール方法

このドキュメントは、複数の Ubuntu 24.04 をインストールしたサーバーに、 marmot をインストールして、クラスタを構成する時の手順です。

前提条件とdebパッケージのインストールまでは、[marmot シングル構成のインストール手順](HOWTO-install-marmot.md)と同じです。
このドキュメントでは、debパッケージをインストールした後、クラスタ構成のために必要な手順を記述します。

## marmot clusterのノード構成

marmotd が動くノードには、Kubernetesと同様に、マスターと単なるノードの２種類があります。
マスターの役割は、mactl コマンドからのリクエストを受けること、サーバー、ネットワーク、ボリュームなどのコントローラーを実行、および、ノードの機能を果たします。
ノードは、仮想マシンの実行、ストレージ管理、OSイメージの管理など、仮想マシンを起動するうえで、必要な機能を担当します。

marmotクラスタ上で作られるオブジェクトの情報は、すべてetcdに保存され、etcd上の情報と、オブジェクトの状態が一致するように制御しています。
マスターと各ノードから、etcdをアクセスしていますので、同じetcdサーバーをアクセスするように、設定する必要があります。

etcdの可用性は、etcdクラスタを構成することで対応を計画していますが、2026/6現在、etcdクラスタ構成には対応していません。



## maadm cluster でクラスタを構成する（推奨）

`maadm cluster` コマンドで、ノードをクラスタへ編入、離脱させることができます。
`maadm` は、marmot デーモンを介さずに、etcd の設定、`/etc/marmot/marmotd.json`、クラスタ情報を変更する管理者用のコマンドで、root 権限が必要です。
いずれのサブコマンドも、`--dry-run` で変更内容の表示だけを行い、`--force` で確認プロンプトを省略できます。
変更するファイルは `*.bak` に退避し、途中で失敗した場合は、`init` と `join` は元のシングルノード構成へ戻します。

### 事前準備

- 全ノードで、deb パッケージのインストールが終わり、シングルノードとして marmot と etcd が動作していること。
- 全ノードで、一意な HOSTID を設定すること（後述の「ベアメタル導入時のユニークな hostid 設定」を参照）。
- 全ノードで、`node_name`（`/etc/marmot/marmotd.json`）が重複していないこと。
- ノード間で、次のポートが通ること。etcd は TCP 2379、OVN SB DB は TCP 6642、Geneve は UDP 6081 です。

### マスターノードで init を実行する

クラスタのコントロールノードにするノードで、`maadm cluster init` を実行します。

```console
root@marmot1:~# maadm cluster init --etcd-ip 172.16.0.201 --underlay-interface enp2s0
以下の変更を行います。
  /etc/systemd/system/etcd.service.d/marmot-cluster.conf: DAEMON_ARGS で etcd を http://172.16.0.201:2379 へ公開
  /etc/marmot/marmotd.json: etcd_url=http://172.16.0.201:2379, default_underlay_interface=enp2s0
  etcd 内: クラスタ情報（CLUSTER-ID, マスター=marmot1）を保存
  etcd.service と marmot.service を再起動します（API が一時的に利用できなくなります。稼働中の VM は停止しません）
続行しますか? [y/N]: y
CLUSTER-ID: 6f1c0c0e-....
etcd: http://172.16.0.201:2379
他のノードで次のコマンドを実行してください:
  maadm cluster join 6f1c0c0e-.... --master-node 172.16.0.201
```

| オプション | 説明 | 既定値 |
|---|---|---|
| `--etcd-ip` | etcd が待ち受け、他ノードへ公開する IPv4 アドレス | `--underlay-interface` の IPv4 アドレス |
| `--underlay-interface` | ノード間のオーバーレイ（Geneve）に使うインターフェース。`default_underlay_interface` に設定する | 現在の `default_underlay_interface`、未設定なら `br0` |

`init` は、次の変更を行います。

- `etcd.service` 本体は変更せず、drop-in（`/etc/systemd/system/etcd.service.d/marmot-cluster.conf`）で `DAEMON_ARGS` を上書きして、etcd を `127.0.0.1` と `--etcd-ip` で待ち受けさせる。
  `etcd.service` や `/etc/default/etcd` で独自の引数を指定している場合は、その引数が上書きされるので、drop-in に含めてください。
  また、`ETCD_LISTEN_CLIENT_URLS` と `ETCD_ADVERTISE_CLIENT_URLS` の環境変数（`override.conf` など）が設定されていると、etcd はフラグと競合して起動できないため、drop-in の `UnsetEnvironment=` で無効化する。該当する変数がある場合は、実行前の確認画面に表示される。
- `marmotd.json` の `etcd_url` と `default_underlay_interface` だけを更新する。他の項目は変更しない。
- CLUSTER-ID（UUID v4）とマスターノード名を etcd に保存する。
- `etcd.service` と `marmot.service` を再起動する。OVN の設定（`ovn-encap-type`、`ovn-encap-ip`、`ovn-remote`）は、起動時に marmotd が自動で行う。

すでにクラスタが有効なノードで再実行しても、何も変更せず、CLUSTER-ID を表示して正常終了します。

### 他のノードで join を実行する

クラスタへ編入するノードで、`init` が表示した CLUSTER-ID と、マスターの IP アドレスを指定して、`maadm cluster join` を実行します。

```console
root@marmot2:~# maadm cluster join 6f1c0c0e-.... --master-node 172.16.0.201 --underlay-interface enp2s0
```

| オプション | 説明 | 既定値 |
|---|---|---|
| `--master-node` | マスターノードの IP アドレス（必須）。etcd のポートは 2379 | なし |
| `--underlay-interface` | ノード間のオーバーレイ（Geneve）に使うインターフェース | `br0` |

`join` は、次の場合に、何も変更せずにエラーで終了します。

- すでにクラスタのメンバーである。
- ローカルの etcd に、VM やネットワークなどの管理対象がある（クラスタへ引き継がれないため。marmotd が自動作成する `mgmt` ネットワークは対象外）。
- マスターへ接続できない、マスターでクラスタが有効になっていない、または CLUSTER-ID が一致しない。
- `node_name` または HOSTID が、すでに登録されているノードと重複している。

事前チェックを通ると、marmot を停止して、ローカルの etcd を無効化（`systemctl disable --now etcd`）し、`marmotd.json` の `etcd_url` をマスターの etcd へ変更してから marmot を起動します。
マスターの etcd にこのノードが登録されたことを確認して、CLUSTER-ID とノード一覧を表示します。登録を確認できない場合は、元のシングルノード構成へ戻します。

### ノードをクラスタから離脱させる

離脱するノードで、`maadm cluster leave` を実行します。

```console
root@marmot2:~# maadm cluster leave 6f1c0c0e-....
```

**このノード上の VM は、シャットダウンして削除され、復元できません。**
必要なデータは、事前にバックアップしてください。実行すると、削除される VM の一覧が表示されます。`--dry-run` で、変更せずに確認できます。

- マスターノードは離脱できません。マスターを外す場合は、先に他のノードを離脱させてください。
- VM の削除には、離脱するノードの `marmot.service` が稼働している必要があります。VM のシャットダウンが 120 秒で完了しない場合は、強制停止して削除します。
- VM を削除できなかった場合は、メンバーのまま中断します。再実行すると、残りの VM から続行します。
- クラスタから自ノードの情報を削除した後、ローカルの etcd を有効にして、`etcd_url` を `http://127.0.0.1:2379` に戻し、OVN の `ovn-remote` と SB DB 上の chassis を削除して、marmot をシングルノードとして起動します。
  OVN のクリーンアップに失敗した場合は、警告と手動コマンドを表示して、離脱は完了します。
- ローカル構成の復元で失敗した場合は、原因を解消して再実行すると、その手順から続行します。
- 再びクラスタへ参加する場合は、`maadm cluster join` を実行します。

### 手動構成のクラスタを maadm の管理へ移行する

以前の手順（次の「手動での構成手順」）で構成したクラスタは、クラスタ情報が etcd にないため、`mactl cluster` がエラーになります。
マスターで `maadm cluster init` を実行すると、クラスタ情報が登録されます（etcd と marmot が再起動されます。稼働中の VM は停止しません）。

## 手動での構成手順（maadm を使わない場合）

以降の手順は、`maadm cluster` を使わずに、手作業でクラスタを構成する場合の手順です。
「ベアメタル導入時のユニークな hostid 設定」は、maadm を使う場合も必要です。
「/etc/resolve.confの編集」「イメージの再ロード」「クラスタ構成の確認」は、どちらの場合も共通です。
手動で構成した場合、`mactl cluster` を使うには、マスターで `maadm cluster init` の実行が必要です。

## インストール後の操作

それぞれのサーバーで、パッケージのインストールが終わったら、marmot と etcd を停止させる。

```console
/tmp# systemctl stop marmot
/tmp# systemctl stop etcd
```

マスター以外のノードは、etcdが起動しないようにします。

```console
/tmp# systemctl disabel etcd
```

## ベアメタル導入時のユニークな hostid 設定

`mactl cluster` の `HOSTID` 列は、各ノードで `hostid` コマンドを実行した値です。
この値は、ネットワークのフェイルオーバー優先順位、IPAM、スケジューラのタイブレークにも
使われるため、クラスタ内の全ノードで一意である必要があります。

クローンしたOSイメージや `/etc/hosts` のホスト名解決が `127.0.1.1` のままのベアメタル機では、
`/etc/hostid` が存在しないと glibc の `gethostid()` がホスト間で同じ値を返し、
複数ノードの HOSTID が重複することがあります。marmot / etcd を起動する前に、
各ノードで一意な `/etc/hostid` を設定してください。

```console
root@marmot1:/tmp# hostid
007f0100
root@marmot1:/tmp# sudo bash -c 'head -c4 /dev/urandom > /etc/hostid'
root@marmot1:/tmp# hostid
61e3eba0
```

上記をクラスタの全ノードで実施し、`hostid` の出力がノードごとに異なることを確認してから、
次の手順に進んでください。

## マスターの etcd を 他ノードからアクセス可能にする

クラスタの他メンバーと etcd を共有できるように、クラスタネットワークのIPアドレスにポートを開くように設定する。

```console
root@marmot1:/tmp# vi  /usr/lib/systemd/system/etcd.service
root@marmot1:/tmp# grep DAEMON_ARGS=  /usr/lib/systemd/system/etcd.service
Environment=DAEMON_ARGS="--listen-client-urls http://172.16.0.201:2379 --advertise-client-urls http://172.16.0.201:2379"
```

変更した設定を再読込してetcdを起動する。

```console
root@marmot1:/tmp# systemctl daemon-reload
root@marmot1:/tmp# systemctl start etcd
root@marmot1:/tmp# systemctl status etcd
● etcd.service - etcd - highly-available key value store
     Loaded: loaded (/usr/lib/systemd/system/etcd.service; enabled; preset: enabled)
     Active: active (running) since Wed 2026-06-10 02:36:24 UTC; 45s ago
       Docs: https://etcd.io/docs
```

## /etc/marmot/marmotd.jsonの編集

マスター側のJSONの etcd のアドレスを、他クラスタのメンバーからアクセス可能なアドレスに変更します。
オーバーレイネットワークのためのインタフェース名をセットします。

```json
{
  "node_name": "marmot1",
  "etcd_url": "http://172.16.0.201:2379",  このアドレスは、クラスタに公開するIPアドレスを設定します。
  "dns_listen_addr": "192.168.1.201:53",　 他のクラスタメンバー、VMからも参照できるアドレスを設定します。
  "default_underlay_interface": "enp2s0",　marmot ノード間で、オーバーレイ・ネットワークに使用する インターフェースです。
　＜他省略＞
}
```

ノード側は、のJSONの etcd のアドレスは、マスターのアドレスと同じアドレスにします。
オーバーレイネットワークのためのインタフェース名をセットします。

```json
{
  "node_name": "marmot2",
  "etcd_url": "http://172.16.0.201:2379",　　マスターのIPアドレスを設定します。
  "dns_listen_addr": "192.168.1.202:53",     他のクラスタメンバー、VMからも参照できるアドレスを設定します。
  "default_underlay_interface": "enp2s0",　　オーバーレイネットワーク用のNICを指定します。
　＜他省略＞
}
```


## /etc/resolve.confの編集

マスターとノードのそれぞれの`/etc/resolv.conf`について設定変更を確認します。

マスター `/etc/resolv.conf`
```yaml
# generate by marmotd
nameserver 192.168.1.201
options edns0 trust-ad
search host-bridge
```

ノード  `/etc/resolv.conf`
```yaml
# generate by marmotd
nameserver 192.168.1.202
options edns0 trust-ad
search host-bridge
```

## イメージの再ロード

仮想マシンのOSイメージがロードに失敗している可能性があります。
問題を解消するには、イメージを削除した後に、`systemctl restart marmot` を実行することで、
クラスタメンバーのイメージを同期させることができます。

```console
$ mactl get image
$ mactl delete image ubuntu22.04
$ mactl delete image ubuntu24.04
$ mactl get image
$ sudo systemctl restart marmot
```


## クラスタ構成の確認

`mactl cluster` コマンドで、CLUSTER-ID とクラスタメンバーを表示することができます。
クラスタが有効でない場合（`maadm cluster init` を実行していない場合）は、エラーになります。

```console
ubuntu@ws1:~$ mactl cluster
ID: 6f1c0c0e-....
NODE             HOSTID     IP              CAP_CPU CAP_MEM(MB)  TOTAL RUNNING STOPPED     VCPU  MEM(MB)    VNETS AGE
marmot1          61e3eba0   172.16.0.201          8      15991      0       0       0        0        0        6 56d
marmot2          9bffb5e3   172.16.0.202          8      15991      0       0       0        0        0        6 56d
marmot3          67aa9474   172.16.0.203          8      15991      0       0       0        0        0        6 56d
```

`-o json` と `-o yaml` では、メンバーの一覧だけが出力されます。
Geneve トンネルの状態は、[HOWTO-trableshort-geneve](HOWTO-trableshort-geneve) の手順で確認します。
