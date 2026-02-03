# Private Micro Cloud "Marmot" 

Marmot（マーモット）は、テスト、学習、実験用に設計された、高速で軽量な仮想サーバーオーケストレーションツールです。
YAMLで仮想サーバーの構成を定義し、KVMやその他のLinuxネイティブテクノロジーを使用して数秒で起動できます。

Marmotという名称は、リリース 1.0 に到達するまでの仮称の予定です。

## 特徴

- 高速起動でOSが起動開始する仮想サーバー
- YAMLベースの構成
- ブートとデータのボリュームに、LVMとQCOW2の選択ができる
- etcd、Open vSwitchなどとの統合
- Ubuntu のサポート
- OpenAPI v3 ベースのREST-APIで MarmotサーバーをAPI操作


## インストール方法

libvirt, etcd,Open vSwitch,LVM,KVM などのインストールと設定の後、以下の要領で、起動することができます。インストールなどのドキュメントは順次拡充していきます。

データベースサーバーに、etcdを使用しています。
hvサーバー個別に、etcdをインストールするときは、以下の手順でインストールしてください。

```
sudo apt update && sudo apt install etcd
systemctl status etcd
```

- [ネットワークの設定方法](docs/network-setup.md)
- [データベースの初期化方法](cmd/hv-admin/README.md)

marmotのインストールは、deb 形式に変えていく予定です。


## 使用例

修正中


## インストール方法

修正中


- [ネットワークの設定方法](docs/network-setup.md)
- [データベースの初期化方法](cmd/hv-admin/README.md)


marmotのダウンロードとインストール

**見直し中**


*応用例*

- [設定用Ansibles集](https://github.com/takara9/marmot-servers)
- [Kubernetesクラスタの実行](https://github.com/takara9/marmot-servers/tree/main/kubernetes)
- [Cephストレージシステムの実行](Https://Github.Com/Takara9/Marmot-servers/tree/main/ceph)
- [メトリックスとログ分析基盤](https://github.com/takara9/docker_and_k8s/tree/main/4-10_Observability)
- [GitHub Actionと連携したmarmot開発環境](https://github.com/takara9/marmot/docs/HOWTO-CI.md)


## アーキテクチャ

**変更中**

mactlコマンドに、仮想マシンのクラスタ構成 YAML を添えて実行することで、仮想マシンが起動します。クラスタは、1サーバーから、リソースのあるだけ起動できます。

![Architecture](docs/architecture-1.png)


複数のmarmotを導入したサーバーを並列化して、クラウドの様な環境を構築できます。

![Architecture](docs/architecture-2.png)


## ライセンス

このプロジェクトはMITライセンスの下で提供されています。詳細は[LICENSE](LICENSE)ファイルをご覧ください。

## 貢献

貢献を歓迎します！ガイドラインについては[CONTRIBUTING.md](CONTRIBUTING.md)をご覧ください。

## 連絡先

メンテナー: [takara9](https://github.com/takara9)
ご質問や議論については、[GitHub Discussions](https://github.com/takara9/marmot/discussions) をご利用ください。