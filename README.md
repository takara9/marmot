# Private Micro Cloud "Marmot" 

Marmot（マーモット）は、テスト、学習、実験用に設計された、高速で軽量な仮想サーバーツールです。
YAMLで仮想サーバーのクラスタ構成を定義し、KVMやその他のLinuxネイティブテクノロジーを使用して数秒で起動できます。

## 特徴

- 高速起動でOSが起動する仮想サーバー
- YAMLベースの構成
- LibVirt、etcd、Open vSwitchなどとの統合
- Ubuntu 20.04以降をサポート
- OpenAPI v3 ベースのREST-APIで MarmotサーバーをAPI操作
- ブートボリューム、データボリュームに、LVMとQCOW2の選択が可能
- LXCの実装も推進中


## 使用例

構成ファイルを準備して、mactlコマンドにファイル名をセットして、仮想マシンが構築できます。

```
$ mactl server create -f config.yaml
```

## インストール方法

作成中
debパッケージでのインストールを提供予定



## *応用例*
もともとは、自身の検証や学習のために作ったソフトウェアです。

- [設定用Ansibles集](https://github.com/takara9/marmot-servers)
- [Kubernetesクラスタの実行](https://github.com/takara9/marmot-servers/tree/main/kubernetes)
- [Cephストレージシステムの実行](Https://Github.Com/Takara9/Marmot-servers/tree/main/ceph)
- [メトリックスとログ分析基盤](https://github.com/takara9/docker_and_k8s/tree/main/4-10_Observability)
- [GitHub Actionと連携したmarmot開発環境](https://github.com/takara9/marmot/docs/HOWTO-CI.md)


## アーキテクチャ
大きく構造を変更中です。以下の図は、これまで採用した進化過程の一つの段階です。

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
