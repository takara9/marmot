# Marmot に可観測性の機能を実装

- VXLANのオーバーレイを廃止して Geneveオーバーレイに限定する。
- 本変更で VXLAN関係のコード、APIは廃止する。

## マネジメント専用ネットワークの開設
- ゲストVMは、マニフェストに指定が無くても、強制的にマネジメント専用ネットワークに接続する。
- クラスタ構成に対応するために、このネットワークはOVN上に構成する。
- apt-cacher-ngをインストールして、マネジメント専用ネットワーク上で、ゲストVMがアクセスできるようにする。
- クラスタ構成の場合、マネジメント専用ネットワークは、geneveを利用して、クラスタ横断で疎通可能でなければならない。
- マネジメント専用ネットワークは、ゲストVM同士の通信は許可されず、Marmotクラスタ上のPrometheus, Loki, DNSなどのサーバーとポート番号など許可された相手先へのみ通信が許可される。
- Marmotd は、ゲストネットワークに接続されるゲストVMのIPアドレスの付与、返却を全て管理する。
- モニタリングの活動は、このマネジメント専用ネットワークを経由して実施される。

## クラスタ構成時の起動について
- 



## 必要プロセスの起動、保存先のディレクトリなど
- Grafana で稼働状態をモニターできるように、Marmotホストで、systemd から起動と停止を実施
- ログは、Lokiを Marmotホストで、systemd から起動と停止を実施
- ログの保存先ディレクトリは、/usr/lib/marmot/logs、デフォルトで4週間保存する。marmotd.jsonの設定で拡大可能とする。
- メトリックスは、PrometheusをMarmotホストで、systemd から起動と停止を実施
- メトリックスの保存先ディレクトリは、/usr/lib/marmot/metrics として、デフォルトで2GBでローテーションする。marmotd.jsonの設定で設定を変えることができる。
- AlertManagerを Marmotホストで、systemd から起動と停止を実施


## 監視対象とゲストOSからのログのPush転送
- ゲストVMとMarmotホストで稼働するLokiは、マネジメント専用ネットワーク上で疎通する。
- ゲストVMログは、Grafana Alloy を利用して、Marmotホストで稼働するLokiへ、ログをプッシュする。

## 監視対象とゲストOSからのメトリックスのPush転送
- ゲストVMとMarmotホストで稼働するPromethusは、マネジメント専用ネットワーク上で疎通する。
- メトリックスは、OpenTelemetry Collectorを利用してPrometheusへプッシュする。

## ゲストOSのログとメトリックスの有効化
- 仮想サーバーのマニフェストで、モニタリングのログをTrue にすることで、ゲストOSをセットアップする際にGrafana Alloyをセットアップする。
- 仮想サーバーのマニフェストで、モニタリングのメトリックスをTrue にすることで、ゲストOSをセットアップする際にOpenTelemetry Collectorをセットアップする。

## アラート通知
- アラート発生時は、marmotd.json にセットした Slackの指定のワークスペースのチャネルへ通知する

## Marmotのダッシュボード WebUI
- GrafanaのWebUIは、host-bridge上にポートを開き、WebUIでアクセス可能とする。
- ホストのメモリ、CPU搭載量に対して、使用率％と実値を表示
- marmot で起動した仮想サーバーの名前、CPU、メモリの割当、使用率などをリアルタイムに表示

## URLアドレスの取得
- mactl status で Grafana, AlertManager のアドレスするための URL が表示する。

## インストーラー
- Grafana, Prometheus, Loki, Node Exporter, Alloy, OpenTelemetry, apt-cacher-ng など必要なソフトウェアをインストールするように追加する。