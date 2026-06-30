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

`mactl marmot cluster` コマンドで、クラスタメンバーを表示することができます。

```console
ubuntu@ws1:~$ mactl marmot cluster
NODE             HOSTID     IP              CAP_CPU CAP_MEM(MB)  TOTAL RUNNING STOPPED     VCPU  MEM(MB)    VNETS UPDATED
marmot1          61e3eba0   172.16.0.201          8      15991      0       0       0        0        0        6 2026-06-30 11:06:31
marmot2          9bffb5e3   172.16.0.202          8      15991      0       0       0        0        0        6 2026-06-30 11:06:31
marmot3          67aa9474   172.16.0.203          8      15991      0       0       0        0        0        6 2026-06-30 11:06:33
```
