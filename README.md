# marmot 仮想サーバーの管理システム

実験や学習のための、簡便で高速な仮想サーバーの構築環境

複数の仮想サーバーを数秒で起動することができる。
1つから複数のハイパーバイザー上で、仮想サーバーのクラスタを起動できる。


## ビルドとインストール

### hv-admin

etcdにハイパーバイザーのリソース情報をセットするコマンド

```
cd cmd/hv-admin
make 
make install
```

ホームディレクトリに`.config_marmot`が配置すること


### vm-client　（mactl)

marmotのコマンドインタフェース

```
cd cmd/vm-client
make
make install
```

ホームディレクトリに`.config_marmot`が配置すること



### vm-server

marmotのバックエンドサーバー

```
cd cmd/vm-server
make
make install
systemctl start marmot
```



## To Do

* CoreDNS と連携させて、起動したVMがDNSで引けるようにする。




## 参考資料
* Call your code from another module, https://go.dev/doc/tutorial/call-module-code

CIが動か無いので調査



## ローカル環境でテスト

GitHub Actions でテストを実行する際は、CoreDNSとetcdのサービスをワークフローの中で、起動します。
しかし、ローカルでする時は、サービスを手作業で起動しなければなりません。そのために、以下のコマンドを実行します。

~~~
docker run -d --name coredns -p 1053:53/udp -v /home/ubuntu/marmot/pkg/dns/testconf:/conf -e CONF="/conf/Corefile" -e PORT="53"  maho/coredns:1.11.1 
docker run -d --name etcd -p 2379:2379 -e ALLOW_NONE_AUTHENTICATION=yes -e ETCD_ADVERTISE_CLIENT_URLS="http://127.0.0.1:2379" bitnami/etcd
~~~

テストが終わった後は、以下のコマンドで、停止させておきます。再スタートするには、`docker start [name]` を実行します。

~~~
docker stop coredns
docker stop etcd
~~~

