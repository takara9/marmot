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
