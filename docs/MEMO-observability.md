# Marmot に可観測性の機能を実装

## マネジメント専用ネットワークの開設
- ゲストVMは、マニフェストに指定が無くても、強制的にマネジメント専用ネットワークに接続する。
- マネジメント専用ネットワークのIPネットは、10.245.0.0/16 を使用する。
- クラスタ構成に対応するために、マネジメント専用ネットワークはOVN上に構成する。
- apt-cacher-ngをインストールして、マネジメント専用ネットワーク上で、ゲストVMがアクセスできるようにする。
- クラスタ構成の場合、マネジメント専用ネットワークは、geneveを利用して、クラスタ横断で疎通可能でなければならない。
- マネジメント専用ネットワークは、ゲストVM同士の通信は許可されず、Marmotクラスタ上のPrometheus, Loki, DNSなどのサーバーとポート番号など許可された相手先へのみ通信が許可される。
- Marmotd は、ゲストネットワークに接続されるゲストVMのIPアドレスの付与、返却を全て管理する。
- モニタリングの活動は、このマネジメント専用ネットワークを経由して実施される。




## Marmotホスト、および,Marmotクラスタメンバーの監視
- 各hostControllerは、以下の監視を実施して、ステータスをデータベースに書き込む
- marmotクラスタの他marmotホストの死活監視 オーバーレイネットワークが使用するI/FのIPアドレスのping監視
- marmotクラスタの他marmotホスト同士のオーバーレイネットワークの活動状況をチェック、Marmotクラスタのステータスに状態をセット


## 監視対象ゲストVMをノードコントローラーが監視

コードの保守性改善のためコントローラーの名前をリファクタリングする。


## 必要プロセスの起動、保存先のディレクトリなど
- Grafana で稼働状態をモニターできるように、Marmotホストで、systemd から起動と停止を実施
- ログは、Lokiを Marmotホストで、systemd から起動と停止を実施
- ログの保存先ディレクトリは、/var/lib/marmot/logs、デフォルトで4週間保存する。marmotd.jsonの設定で拡大可能とする。
- メトリックスは、PrometheusをMarmotホストで、systemd から起動と停止を実施
- メトリックスの保存先ディレクトリは、/var/lib/marmot/metrics として、デフォルトで2GBでローテーションする。marmotd.jsonの設定で設定を変えることができる。
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



Q1 マネジメントネットワークの予約名（mgmt 等）でよいか:  mgmtで良い
Q2 ACLの許可リストの設定方法（marmotd.json に静的に書く／DBに登録する動的サービスディスカバリか）:  marmotd.json に静的に書く
Q3 既存の default/host-bridge 以外に複数NIC強制付与した場合の、ディスクバス番号との衝突回避ロジック（server.go:649 のbusno計算）への影響有無: 無
Q4 既存ゲスト（稼働中VM）への遡及適用は範囲外として良いか（新規作成のみ対象）: 遡及適用は範囲外


実装方針（フェーズ分割案）
大きな機能のため、1変更セット=1フェーズで段階的に進めることを提案します。

フェーズ1: マネジメントネットワークの自動作成とIPAM

marmotd 起動時（またはクラスタ初期化時）に予約名（例: mgmt）のネットワークを 10.245.0.0/16 で自動作成
overlay は geneve 固定、既存の applyVirtualNetworkDefaults を流用
host-bridge と同様に isIPAMUnmanagedNetwork 相当の予約名判定を追加
フェーズ2: ゲストVMへの強制アタッチ

server.go:430 のNIC組み立てロジックを変更し、マニフェストの NetworkInterface に関わらず、マネジメントネットワーク用NICを常に追加
ゲストOS側のnetplan/interfaces生成（setup-linux.go）にも対応が必要
フェーズ3: ACLによる通信制御

OVN ACL（ovn-nbctl acl-add）を使い、マネジメントネットワークのlogical switch上で「ゲスト間拒否・許可リスト宛のみ許可」を実装
許可リスト（Prometheus/Loki/DNSサーバーのIP:ポート）は marmotd.json などで設定可能にする想定
フェーズ4: apt-cacher-ng 等インストーラー対応

Marmotホスト側に apt-cacher-ng をセットアップし、マネジメントネットワーク経由でゲストVMからアクセス可能にする（ACL許可リストにも追加）
フェーズ5: クラスタ横断疎通の検証

既存の OVNFabric.EnsureOverlayMesh がクラスタ複数ノード間のgeneveメッシュを構築済みのため、マネジメントネットワークもこの仕組みに乗せられるか検証
オープン事項（着手前に確認したいこと）
マネジメントネットワークの予約名（mgmt 等）でよいか
ACLの許可リストの設定方法（marmotd.json に静的に書く／DBに登録する動的サービスディスカバリか）
既存の default/host-bridge 以外に複数NIC強制付与した場合の、ディスクバス番号との衝突回避ロジック（server.go:649 のbusno計算）への影響有無
既存ゲスト（稼働中VM）への遡及適用は範囲外として良いか（新規作成のみ対象）



1. 許可リストのconfig項目名・スキーマは上記案(management_network_acl_allow: description/cidr/protocol/port の配列)でよいか: 良い
2. デフォルトルール優先度(2000=許可, 1000=拒否)の考え方でよいか: 良い
3. to-lport(Marmotホスト→ゲスト方向)の制御は今回スコープ外でよいか(Push型のみ対応): 良い
4. marmotd.jsonの初期値(config-sample)は空リスト([])でよいか、それとも代表例(Prometheus/Loki/DNSの想定ポート)を仮で入れておくか: 良い

2. デフォルトルール優先度(2000=許可, 1000=拒否)の考え方でよいか: 良い


To Do リストを更新しました

`marmotd.json` に以下のように追記する形になります（`marmotd-config.go`の`ManagementNetworkACLAllowEntry`構造体に対応）。

```json
{
  "management_network_acl_allow": [
    {
      "description": "prometheus",
      "cidr": "10.245.0.1/32",
      "protocol": "tcp",
      "port": 9090
    },
    {
      "description": "loki",
      "cidr": "10.245.0.1/32",
      "protocol": "tcp",
      "port": 3100
    },
    {
      "description": "dns",
      "cidr": "10.245.0.1/32",
      "protocol": "udp",
      "port": 53
    }
  ]
}
```

## 各項目の意味

| 項目 | 型 | 説明 |
|---|---|---|
| `description` | string | 用途を表す任意のラベル（動作には影響しない、ログ表示用） |
| `cidr` | string | 許可する宛先。単一ホストなら `10.245.0.1/32`（`10.245.0.1` のようにマスク省略も可、内部で自動的に`/32`扱いになる）、サブネット単位なら `10.245.0.0/24` のような指定も可能 |
| `protocol` | string | `tcp` または `udp`（大文字小文字は区別しない）。これ以外は不正なエントリとして無視され、警告ログが出る |
| `port` | int | 許可する宛先ポート番号(1〜65535)。範囲外は不正なエントリとして無視される |

このリストに列挙されていない宛先・ポートへのゲストVMからの通信は、`mgmt`ネットワーク上で全て拒否されます（ゲストVM同士の通信も含む）。未設定（`[]`または省略）の場合は、全ての発信通信が拒否される状態になります。

To Do リストを更新しました

追加の設定は不要です。フェーズ3で実装した内容により、`ping 10.245.0.3`(ゲストVM間の通信)は**既に自動的に遮断されます**。

## 理由

現在のACL設計は以下の2段階です。

```
優先度2000: 許可リストに一致する通信(tcp/udp + 宛先ポート指定)のみ → allow-related
優先度1000: それ以外の全てのIPv4通信(ip4) → drop
```

ping(ICMPプロトコル)は、`protocol: tcp` または `protocol: udp` を条件とする許可リストのルール(`tcp.dst==...`/`udp.dst==...`)には**どのIPv4パケットに対しても一致しません**（ICMPパケットにはTCP/UDPポートの概念が無いため）。そのため:

- ゲストVM同士のping → 優先度2000の許可ルールに一致せず、優先度1000の `ip4` (=全IPv4通信対象)のdropルールにのみ一致 → **遮断される**
- 許可リストに列挙したPrometheus等へのtcp/udp通信 → 優先度2000のallow-relatedルールに一致 → 許可される

つまり、`management_network_acl_allow` に何も追記しなければ、mgmtネットワーク上のゲストVM同士の通信（pingを含む全プロトコル）はデフォルトで遮断される設計になっています。

## 注意点（現状の制約）

もし逆に「Marmotホストの監視サーバーへのpingだけは許可したい」といったICMPの許可が必要になった場合、現在の`ManagementNetworkACLAllowEntry`スキーマは `protocol` に `tcp`/`udp` のみを受け付ける設計のため、ICMPの許可には対応していません。その場合はスキーマ拡張（`protocol: icmp` でポート指定なしの許可ルールを組み立てる等）が別途必要になります。今は不要であれば対応しませんが、必要になれば教えてください。