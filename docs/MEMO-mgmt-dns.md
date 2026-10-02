# mgmtネットワーク経由のDNS名前解決（設計メモ）

関連: [MEMO-observability.md](./MEMO-observability.md)

## 背景・要求

[MEMO-observability.md](./MEMO-observability.md) には以下の記述がある。

> マネジメント専用ネットワークは、ゲストVM同士の通信は許可されず、Marmotクラスタ上のPrometheus, Loki, DNSなどのサーバーとポート番号など許可された相手先へのみ通信が許可される。

これは、ゲストVMが `mgmt` ネットワーク経由で `10.245.0.1:53`（Marmotホスト自身）へDNSクエリを送り、名前解決できることを期待している。本メモは、この要求に対する現状の実装状況のギャップと、実現するための設計案を整理する。

## 現状の実装状況（2026-10-01 時点）

### 既にあるもの

- `mgmt` ネットワーク（予約名、`10.245.0.0/16`、OVN上に構成）の自動作成
  - [pkg/marmotd/network_management.go](../pkg/marmotd/network_management.go) `EnsureManagementNetwork()`
- ゲストVMへの `mgmt` NICの強制アタッチ
  - [pkg/marmotd/network_management.go](../pkg/marmotd/network_management.go) `attachManagementNetworkInterface()`
- OVN ACLによる通信制御（許可リスト以外はゲスト間含め全遮断）
  - [pkg/marmotd/marmotd-config.go](../pkg/marmotd/marmotd-config.go) `ManagementNetworkACLAllowEntry` / `management_network_acl_allow`
  - [pkg/marmotd/network_management_acl_test.go](../pkg/marmotd/network_management_acl_test.go) `BuildManagementNetworkACLRules`
- Marmotホスト自身を `mgmt` ネットワーク上で到達可能にする `mgmt-host` ポート（`10.245.0.1/16`）
  - [pkg/networkfabric/ovn_host_presence.go](../pkg/networkfabric/ovn_host_presence.go) `EnsureHostPresencePort()`
  - 呼び出し元: [pkg/controller/network-controller.go](../pkg/controller/network-controller.go)（ネットワークコントローラーの定期ループ内、5秒間隔）
- apt-cacher-ng連携（`mgmt-host` 経由でゲストVMからアクセス可能、ACL許可リストにも登録済み）
- marmotd内蔵のDNSサーバー（`internal-dns`）
  - [pkg/internal-dns/dns-server.go](../pkg/internal-dns/dns-server.go)
  - etcd登録済みの内部レコードを解決し、無ければ `dns_upstream` へフォワードするキャッシュ/フォワード型
  - リッスンアドレスは `marmotd.json` の `dns_listen_addr`（既定運用では host-bridge 側の公開IPや `127.0.0.1`）

### 未実装・ギャップ

| # | ギャップ | 詳細 |
|---|---|---|
| 1 | ACL許可リストにDNS(53/udp)が無い | [cmd/marmotd/marmotd.json](../cmd/marmotd/marmotd.json)・[config-sample/marmotd.json](../config-sample/marmotd.json) の `management_network_acl_allow` には `apt-cacher-ng`(3142/tcp) のみ登録されており、53/udpへの許可エントリが存在しない。現状のままではゲストVMからmgmt経由のDNS問い合わせは遮断される。 |
| 2 | DNSサーバーが `10.245.0.1` にバインドされる保証がない | `internal-dns` は `cfg.DNSListenAddr` にbindするが、この値はhost-bridge側の公開IPとして運用されている（[tools/deb/postinst](../tools/deb/postinst) でも host-bridge IP に書き換え）。`mgmt-host` インターフェース専用のbindにはなっていない。 |
| 3 | 起動順序の競合（レース） | `internaldns.StartInternalDNSServer()` は [cmd/marmotd/marmotd-main.go](../cmd/marmotd/marmotd-main.go) で起動直後に同期的に `ListenPacket` を呼ぶ。一方 `mgmt-host` ポート（`10.245.0.1`の実体）はネットワークコントローラーの定期ループ（`NETWORK_CONTROLLER_INTERVAL = 5秒`、[pkg/controller/network-controller.go](../pkg/controller/network-controller.go)）内で非同期に作成される。`dns_listen_addr` を `10.245.0.1:53` に変更しても、起動直後は `mgmt-host` がまだ存在せず bind に失敗し得る。bind失敗時のリトライは無く、marmotdプロセス自体が起動失敗する。 |
| 4 | ゲストのnameserver設定が `10.245.0.1` を向いていない | ゲストVMに配布するnetplan設定を組み立てる `defaultNameserversFromConfig()`（[pkg/marmotd/server.go](../pkg/marmotd/server.go)）は `dns_listen_addr` / `dns_upstream` のIPから構成しており、`ManagementNetworkHostAddress`（`10.245.0.1`）を参照するロジックが無い。 |

## 設計案（採用: 案B）

### 案A: 既存 `internal-dns` のリッスンアドレスを `mgmt-host` 起動後に切り替える（不採用）

- `internal-dns` の起動を `EnsureHostPresencePort` 完了（`mgmt-host` ポート作成）後まで遅延させる、またはbind失敗時にリトライするロジックを追加する。
- `dns_listen_addr` とは別に `ManagementNetworkHostAddress`（`10.245.0.1:53`）用の第2リスナーを同一プロセス内に追加する案も考えられる（既存の公開IP向けリスナーは維持しつつ、mgmt専用リスナーを追加）。
- 長所: 新しい常駐プロセスが不要。内部レコード解決ロジックをそのまま共用できる。
- 短所: marmotdの起動シーケンス・ネットワークコントローラーとの連携箇所に手を入れる必要がある。

### 案B: mgmt専用の軽量キャッシュ/フォワード用リスナーを追加する（採用）

- `mgmt-host` ポートの準備完了イベントを契機に、`10.245.0.1:53` 専用のUDPリスナーを起動し、既存 `internal-dns`（内部レコード解決＋上位フォワード機能を持つ）へクエリを転送する。
- 長所: 既存の公開向けDNSサーバーの挙動に影響を与えずに追加できる。
- 短所: 構成要素が増える（プロセス内に新しいリスナーを足す場合でも、起動順序の課題は案Aと同様に残る）。
- `mgmt-host` ポート作成前にゲストVMが先にDNSクエリを送った場合は、一時的な名前解決失敗を許容する（ゲスト起動のブロックや同期待ちは行わない。ゲストOS側の再試行に委ねる）。

共通して必要な対応。

1. `management_network_acl_allow` の既定値に `udp/53` 宛（`10.245.0.1/32`）の許可エントリを追加する。
2. ゲストVMのnameserver設定を `10.245.0.1` に向くよう `defaultNameserversFromConfig()` 等を変更する。
3. フォワード先（上位DNS）の選定: 内部ホスト名解決を維持するため、直接外部DNS（`dns_upstream`）ではなく、既存 `internal-dns` の内部解決＋フォールバック機構を経由させる。

## 実装状況（2026-10-01 時点）

- フェーズ1: 完了
  - mgmt専用DNSフォワーダーを新規実装し、`mgmt-host` ポート準備完了後に起動するようフック
    - [pkg/marmotd/network_management_dns.go](../pkg/marmotd/network_management_dns.go) `EnsureManagementDNSForwarder()`
    - 呼び出し元: [pkg/controller/network-controller.go](../pkg/controller/network-controller.go)（`EnsureHostPresencePort` 成功後）
  - `management_network_acl_allow` の既定値に `dns`(udp/53, `10.245.0.1/32`) エントリを追加
    - [cmd/marmotd/marmotd.json](../cmd/marmotd/marmotd.json)、[config-sample/marmotd.json](../config-sample/marmotd.json)
- フェーズ2: 完了
  - mgmt NICのnameserverを `10.245.0.1`（mgmt専用DNSフォワーダー）に固定
    - [pkg/marmotd/network_management_dns.go](../pkg/marmotd/network_management_dns.go) `managementNetworkNameserversFromConfig()`
    - 呼び出し元: [pkg/marmotd/network_management.go](../pkg/marmotd/network_management.go) `attachManagementNetworkInterface()`（IPネットワーク側で明示指定が無い場合のフォールバック）
    - host-bridge NIC向けの `defaultNameserversFromConfig()` の挙動は変更していない

## オープン事項（回答済み）

1. 案A / 案B のどちらを採用するか → **案Bを採用**（mgmt専用の軽量キャッシュ/フォワード用リスナーを追加）
2. `mgmt-host` ポート作成前にゲストVMが先に起動してDNSクエリを送った場合の許容挙動 → **一時的解決失敗を許容**する（リトライ・同期待ち機構は設けない）
3. ACL許可リストの既定値に `dns`(udp/53, `10.245.0.1/32`) エントリを追加するか → **追加する**
4. 本対応のスコープ（新規作成ゲストのみか、既存ゲストへの遡及適用は行うか）→ **新規ゲストのみ**（既存ゲストへの遡及適用は範囲外）
