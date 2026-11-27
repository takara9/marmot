# Marmot コントローラーの課題・改善候補

この文書は、現在の実装から読み取れる運用上のリスクと保守性の改善候補をまとめたものです。提案はコード上の挙動を根拠にしていますが、すべてが再現確認済みの障害という意味ではありません。修正に着手する際は、影響範囲と既存仕様を確認し、回帰テストを追加してください。

優先度の目安:

- **高**: 条件によってデータ消失、サービス中断、またはリソース収束の停止につながる可能性がある。
- **中**: 復旧遅延、不要なリソース残存、または規模拡大時の性能・運用負荷につながる可能性がある。
- **低**: 可観測性やコード保守性の改善。

## 共通の課題

1. **周期ポーリングと全件処理の拡張性** — 多くのループが数秒ごとにリソース一覧を取得して順次処理します。リソース数や外部操作時間が増えると、変更検知の遅延や 1 回の reconcile の長時間化につながります。etcd watch / work queue の導入、処理時間・キュー長の計測、上限付き並列処理を段階的に検討します。例: [scheduler-controller.go](../pkg/controller/scheduler-controller.go#L73)、[network-controller.go](../pkg/controller/network-controller.go#L105)。
2. **状態書き込み失敗の扱いが不統一** — 一部の遷移では DB 更新エラーを無視しています。外部操作の成功後に状態だけ更新できない場合、実体と etcd の状態が食い違い、次の reconcile が二重実行・誤判定する可能性があります。ステータス更新の失敗を共通のログ／メトリクスへ記録し、必要な操作を安全に再実行できるようにします。例: [volume-controller.go](../pkg/controller/volume-controller.go#L203)、[gateway-controller.go](../pkg/controller/gateway-controller.go#L360)、[kubernetes-engine-controller.go](../pkg/controller/kubernetes-engine-controller.go#L226)。
3. **複数ステップ操作の進捗可視性** — VM 作成・Ansible・IPAM 解放など複数の副作用がある処理は、段階ごとの完了状態や所要時間を一貫して追跡できる仕組みがあると、障害時の原因特定と安全な再試行が容易になります。

## コントローラー別

### VM（Server）コントローラー

- **高 — VM 削除に失敗しても Server 定義の削除まで進み得る**: `DeleteServerByIdManage` がエラーを返すと `ERROR` を記録しますが、その後も後片付けと Server 定義の削除へ進みます（[server-controller.go](../pkg/controller/server-controller.go#L243)、[server-controller.go](../pkg/controller/server-controller.go#L244)、[server-controller.go](../pkg/controller/server-controller.go#L295)）。VM が残ったまま定義を失う可能性があるため、VM 削除が失敗した場合は定義削除を保留し、再試行できる状態を維持します。
- **中 — 削除時の後片付け失敗が残留リソースになり得る**: IP 解放、OVN 論理ポート削除、DNS エントリー削除に失敗しても処理を続けます（[server-controller.go](../pkg/controller/server-controller.go#L253)、[server-controller.go](../pkg/controller/server-controller.go#L265)、[server-controller.go](../pkg/controller/server-controller.go#L278)）。後片付けの成否を記録し、失敗した副作用を安全に再実行できる状態を残す方法を検討します。
- **中 — 削除済み VM の cloud-init ISO とディレクトリが残留する**: 作成時に `/var/lib/marmot/isos/<Server ID>/cloud-init.iso` を生成し（[server.go](../pkg/marmotd/server.go#L886)、[server.go](../pkg/marmotd/server.go#L894)、[cloud-init.go](../pkg/marmotd/cloud-init.go#L69)）、VM の CD-ROM として接続します（[server.go](../pkg/marmotd/server.go#L913)）。削除処理は libvirt ドメイン削除とボリュームの削除予約を行いますが、この ISO を削除しません（[server.go](../pkg/marmotd/server.go#L1701)、[libvirtXml2.go](../pkg/virt/libvirtXml2.go#L727)）。Server 定義を削除した後は ID から ISO を回収する契機もなく、VM 作成失敗後にも生成済み ISO が残る可能性があります。VM 削除の後片付けに ISO と空ディレクトリの削除を加え、失敗時に再試行できるようにします。既存の残留分は、稼働 VM の CD-ROM 参照や Server ID と照合してから安全に回収します。
  - **対応状況（対応済み）**: 削除時（`DELETING`）に cloud-init ISO と空になったディレクトリを削除するようにしました（[server-controller.go](../pkg/controller/server-controller.go#L260)、[server.go](../pkg/marmotd/server.go#L1726)）。ISO の削除は、libvirt ドメインの削除成功または不存在を確認できた場合に限ります。確認できない場合や ISO 削除に失敗した場合は Server 定義を残して `DELETING` のまま再試行し、IP 解放は ISO 削除の成功後に行います。VM 作成が起動前に失敗した場合は生成済み ISO を削除し、VM 起動後の処理で失敗した場合は稼働中の VM が参照しているため残します（[server.go](../pkg/marmotd/server.go#L892)）。想定外のファイルが残るディレクトリは削除しません。単体テストは [cloud-init_cleanup_test.go](../pkg/marmotd/cloud-init_cleanup_test.go) に追加しました。**注意**: すでに残留している ISO の一括回収は含まれません。
- **中 — `PROVISIONING` の再起動復旧が手動対応**: 起動後に `PROVISIONING` が見つかると `ERROR` に遷移し、自動再開しないメッセージを設定します（[server-controller.go](../pkg/controller/server-controller.go#L304)）。作成工程を冪等に再開できるチェックポイント、または安全に再作成できる復旧手順を整備します。
- **高 — 外部から停止・削除した VM の実状態を `RUNNING` と照合しない**: `RUNNING` 分岐は `SyncServerResourcesManage` を呼びますが（[server-controller.go](../pkg/controller/server-controller.go#L192)）、CPU / Memory の両方が未指定なら同関数は libvirt を確認せず戻ります（[server.go](../pkg/marmotd/server.go#L1814)）。指定がある場合も確認するのはドメイン定義 XML の CPU / Memory 差分であり（[libvirtXml2.go](../pkg/virt/libvirtXml2.go#L977)、[libvirtXml2.go](../pkg/virt/libvirtXml2.go#L983)）、稼働状態ではありません。そのため `virsh destroy` で停止して定義が残った VM は、設定差分がなければ `RUNNING` のままです。`virsh undefine` で定義も消えた場合は、CPU / Memory 指定があると確認エラーから `ERROR` になる一方、両方未指定なら検知されません。定期的にドメインの定義有無と実行状態を観測し、外部停止時に自動再起動するのか `STOPPED` / `ERROR` として報告するのか、期待状態の方針を定義します。
  - **対応状況（対応済み）**: `RUNNING` の Server を libvirt の実状態と照合するようにしました（[server.go](../pkg/marmotd/server.go#L1967)、[libvirtXml2.go](../pkg/virt/libvirtXml2.go#L739)）。CPU / Memory 未指定でも検知します。外部停止は `STOPPED`、ドメイン定義の消失は `ERROR` に更新します。`STOPPED` の Server が外部操作（`virsh start` など）で起動されていた場合は `RUNNING` に更新します（[server.go](../pkg/marmotd/server.go#L2014)、[server-controller.go](../pkg/controller/server-controller.go#L213)）。外部停止・外部起動は Warn ログに記録します。実状態を取得できない場合は状態を変更せず、次の周期で再確認します。状態の更新は、観測時の状態とインスタンス名が一致する場合に限り行うため（[db-servers.go](../pkg/db/db-servers.go#L256)）、API が受理した `STARTING` / `STOPPING` などを上書きしません。判定ロジックの単体テストは [server_running_domain_test.go](../pkg/marmotd/server_running_domain_test.go) と [libvirtXml2_domain_state_test.go](../pkg/virt/libvirtXml2_domain_state_test.go) に追加しました。**制約**: 自動再起動はしません。autostart は変更しないため、API で停止した後に外部から起動した場合、autostart は無効のままです。一時停止・クラッシュなど停止と断定できない状態は、状態を変更しません。

### Volume コントローラー

- **高 — `ERROR` / `PROVISIONING` の古い Volume を自動削除する**: 最終更新から 10 分以上経過した対象は `DeletionTimeStamp` が付与され、その後 `DELETING` の処理で実体も削除されます（[volume-controller.go](../pkg/controller/volume-controller.go#L184)、[volume-controller.go](../pkg/controller/volume-controller.go#L176)）。この判定には永続ボリュームの除外条件が見当たらないため、意図的に長期間エラー状態に置いたデータまで削除対象になる可能性があります。自動削除の適用条件、永続性の扱い、猶予時間、管理者による停止・承認方法を明示し、永続データの保持を優先するテストを追加します。
- **中 — 状態更新エラーが処理継続を妨げない**: `PROVISIONING`、`AVAILABLE`、削除タイムスタンプなどの更新結果を一部確認していません（[volume-controller.go](../pkg/controller/volume-controller.go#L178)、[volume-controller.go](../pkg/controller/volume-controller.go#L203)、[volume-controller.go](../pkg/controller/volume-controller.go#L221)）。DB 更新が失敗したときに、後続の実体操作を続けるかどうかを明示的に定義します。

### Network コントローラー

- **高 — オーバーレイ再調整失敗後に同じ変更を再試行しない場合がある**: メンバー変更を検出すると、再調整を実行する前に `lastNetworkMemberSignature` を新しい値に更新します。再調整が失敗しても署名は戻されず、次周期では変更なしと判定されて再試行されません（[network-controller.go](../pkg/controller/network-controller.go#L130)、[network-controller.go](../pkg/controller/network-controller.go#L137)）。再調整成功後にのみ署名を確定するか、失敗時に再試行対象として残すようにします。
- **中 — 削除依存関係の検査が全件一覧に依存する**: ネットワーク削除前に Server、ALB、Gateway、VPN Gateway の一覧を取得して依存関係を調べます（[network-controller.go](../pkg/controller/network-controller.go#L790)）。対象数の増加で削除処理が重くなるため、etcd のインデックス／キー設計や依存リソースの逆引き方法を検討します。

### Host コントローラー

- **中 — ホスト状態の更新失敗で OVN bootstrap の再試行もスキップする**: 定期処理では HostStatus の収集・更新が失敗すると戻り、後続の OVN bootstrap を実行しません（[host-controller.go](../pkg/controller/host-controller.go#L81)）。両処理を独立させ、片方の一時障害で他方の復旧が妨げられないようにします。
- **低 — 初回 HostStatus の失敗はログのみで起動を継続する**: 初回収集エラーでもサービスは続行します（[host-controller.go](../pkg/controller/host-controller.go#L35)）。起動後、状態が正常に登録されるまでの readiness / health を明示すると、スケジューラーや運用監視が初期化途中のノードを判別しやすくなります。

### Scheduler コントローラー

- **中 — ノード負荷を Server 台数だけで評価する**: `buildNodeLoads` は削除中でない Server の数を数え、最少台数のノードへ割り当てます（[scheduler-controller.go](../pkg/controller/scheduler-controller.go#L167)、[scheduler-controller.go](../pkg/controller/scheduler-controller.go#L193)）。CPU / メモリ / ストレージ容量や Server 要求量を考慮しないため、ノードごとの容量差が大きい環境では偏りが残ります。まず配置制約と容量の情報源を明文化し、容量を考慮したスコアリングを検討します。
- **低 — リーダーは HostStatus から毎周期決定される**: リーダー確認はアクティブホスト状態に基づきます（[scheduler-controller.go](../pkg/controller/scheduler-controller.go#L76)、[scheduler.go](../pkg/marmotd/scheduler.go#L35)）。ホスト状態の更新遅延時にも二重実行を防ぎたい場合は、etcd のリース／ロックによるリーダーフェンシングを検討します。

### Image コントローラー

- **中 — 非同期作成中の Image は再起動後に自動再開しない**: `PENDING` から `CREATING` に変更して作成処理を goroutine で起動しますが、後続の `CREATING` 分岐はログのみです（[image-controller.go](../pkg/controller/image-controller.go#L151)、[image-controller.go](../pkg/controller/image-controller.go#L160)、[image-controller.go](../pkg/controller/image-controller.go#L190)）。プロセス終了で goroutine が失われた場合に作成が滞留するため、処理の永続化、起動時の再開判定、またはタイムアウト後の明示的な失敗遷移を設けます。
- **中 — 作成 goroutine の並列数に上限がない**: 同一ループ内で複数の `PENDING` Image があればそれぞれ goroutine を起動します（[image-controller.go](../pkg/controller/image-controller.go#L133)）。大量同時登録時の帯域・CPU・ディスク負荷を抑えるため、並列数制御とキャンセル可能なワークキューを検討します。

### Gateway コントローラー

- **中 — `ACTIVE` は希望設定のハッシュを照合し、実機の設定状態までは確認しない**: 適用済みハッシュと desired hash の差は検知しますが（[gateway-controller.go](../pkg/controller/gateway-controller.go#L355)）、iptables や systemd の実際の状態を周期的に検証する処理はこの分岐にありません。手動変更や VM 内の状態消失も復旧したい場合は、軽量な実状態チェックと再適用を追加します。
- **低 — 状態更新のエラーを無視する箇所がある**: 特に `ACTIVE` から `CONFIGURING` への遷移は更新結果を確認していません（[gateway-controller.go](../pkg/controller/gateway-controller.go#L360)）。遷移失敗を記録し、次の reconcile で再試行できるようにします。

### VPN Gateway コントローラー

- **中 — `ACTIVE` の実状態検証が desired hash 中心**: 設定ハッシュの差は検知しますが（[vpn-gateway-controller.go](../pkg/controller/vpn-gateway-controller.go#L395)）、OpenVPN サービスの稼働や実設定の健全性をこの処理では検査しません。VPN の稼働状態を表す観測結果を追加し、停止・設定消失を検知したときの再構成方針を定義します。
- **低 — 状態更新のエラーを無視する箇所がある**: 状態遷移の DB 更新結果を複数箇所で破棄しています（[vpn-gateway-controller.go](../pkg/controller/vpn-gateway-controller.go#L176)、[vpn-gateway-controller.go](../pkg/controller/vpn-gateway-controller.go#L395)）。Gateway と同様に永続化失敗を可観測にします。

### Application Load Balancer コントローラー

- **中 — desired 設定の適用と実サービスの健全性確認が分かれている**: コントローラーは agent の state を読み、`LastAppliedHash` や `LastError` を使って状態を更新します（[application-load-balancer-controller.go](../pkg/controller/application-load-balancer-controller.go#L759)）。agent が「適用済み」を報告した後に HAProxy プロセスが停止した場合も検知できるよう、プロセス／ヘルスチェック結果を state に含めることを検討します。
- **低 — agent 状態の観測中に行う一部状態更新の失敗が見えにくい**: 失敗回数や状態遷移の更新結果を無視する箇所があります（[application-load-balancer-controller.go](../pkg/controller/application-load-balancer-controller.go#L760)）。DB に状態を書けない場合も運用者が検知できるログ・メトリクスを整えます。

### Network Load Balancer コントローラー

- **中 — `ACTIVE` は設定ハッシュで desired state を比較するが、iptables の実状態は直接確認しない**: 設定差分が見つかった場合は再構成します（[network-load-balancer-controller.go](../pkg/controller/network-load-balancer-controller.go#L395)）。ノード再起動や手動変更後に実際のルールが失われてもハッシュが一致する場合は検知できない可能性があるため、実ルールのチェックと復旧を検討します。
- **低 — 多くの状態更新結果が無視される**: 例としてバックエンド不在時の `DEGRADED` 遷移や適用後の `ACTIVE` 遷移があります（[network-load-balancer-controller.go](../pkg/controller/network-load-balancer-controller.go#L326)、[network-load-balancer-controller.go](../pkg/controller/network-load-balancer-controller.go#L351)）。状態永続化の失敗を一貫して記録します。

### Kubernetes Engine（MKE）コントローラー

- **中 — コントローラーの説明コメントが実装に追いついていない**: 型コメントは「実際のクラスタ構築は今後実装」と記述していますが（[kubernetes-engine-controller.go](../pkg/controller/kubernetes-engine-controller.go#L40)）、Provisioning では control plane、LB、ノードを構築して Ready を待ちます（[kubernetes-engine-controller.go](../pkg/controller/kubernetes-engine-controller.go#L226)）。コードコメントを現行機能に合わせ、将来の仕様判断で誤解されないようにします。
- **中 — 一時障害と恒久障害の分類・再試行上限が明確でない**: Provisioning / Upgrading で失敗をメッセージに記録して同じ状態に留める経路があります（[kubernetes-engine-controller.go](../pkg/controller/kubernetes-engine-controller.go#L226)、[kubernetes-engine-controller.go](../pkg/controller/kubernetes-engine-controller.go#L378)）。一時的な障害は再試行し、入力不備など恒久障害は `FAILED` にする分類、回数・待機時間、運用者による再開手段を定義します。
- **低 — 削除時の VIP 解放はベストエフォート**: VIP 解放エラーを記録して削除フローを続けます（[kubernetes-engine-controller.go](../pkg/controller/kubernetes-engine-controller.go#L501)）。失敗が後から回収可能かを確認できる残タスク記録や、削除完了後の孤立 VIP 検査を検討します。

### 内部 DNS サーバー

- **中 — DNS 問い合わせごとに etcd を検索する**: `handleRequest` は問い合わせごとに lookup key を順番に etcd へ照会します（[dns-server.go](../pkg/internal-dns/dns-server.go#L148)、[dns-server.go](../pkg/internal-dns/dns-server.go#L166)）。問い合わせ量が増えると etcd 負荷と名前解決遅延に影響するため、更新通知と整合する短期キャッシュ、または watch で更新されるローカル索引を検討します。
- **低 — 正常な名前解決も毎回標準ログへ出力する**: A レコードを etcd から返すたび `log.Printf` しています（[dns-server.go](../pkg/internal-dns/dns-server.go#L187)）。高 QPS 時のログ量を抑えられるよう、構造化ログのレベル制御や集計メトリクスへ移行します。

## marmotd 外の補助コントローラー

### mke-node-controller

- **中 — Service の削除を監視する状態がプロセス内メモリのみ**: 削除済み Service の VIP 解放は `knownServiceVIPs` に前回観測した Service が残っている場合に行います（[reconcile.go](../cmd/mke-node-controller/reconcile.go#L93)、[reconcile.go](../cmd/mke-node-controller/reconcile.go#L121)）。コントローラー停止中に Service が削除されると、再起動後の空の map では対象を特定できず VIP が残る可能性があります。IPAM 側からこのクラスタが管理する VIP を列挙して照合するか、削除を耐久化する仕組みを検討します。
- **低 — Node と LoadBalancer の reconcile を直列実行する**: ループで Node 調整の後に Service LoadBalancer 調整を呼び出します（[main.go](../cmd/mke-node-controller/main.go#L73)）。Node 側の API 呼び出しが遅い場合に VIP 処理も遅れるため、処理時間の計測とタイムアウトを設けます。

### mke-lb-controller

- **高 — Node 一覧取得に失敗しても HAProxy 設定を適用し得る**: `ListNodes` 失敗時はログを出すだけで処理を続け（[main.go](../cmd/mke-lb-controller/main.go#L68)）、そのまま HAProxy 設定を生成・適用します（[main.go](../cmd/mke-lb-controller/main.go#L170)）。一時的な kube-apiserver 障害で空の backend 構成を適用し、既存トラフィックを失う可能性があります。必要な入力の取得に失敗した場合は更新を中止して最後の正常設定を保持します。
- **中 — Host-bridge 情報取得失敗時に backend 候補を空にする可能性がある**: 取得失敗を記録した後も、取得できたアドレスだけで `haproxyNodes` を再構成します（[main.go](../cmd/mke-lb-controller/main.go#L80)）。不完全な観測結果で設定を更新しないための最小件数・鮮度判定と、前回設定を保持する条件を定義します。

### marmot-lb-agent

- **中 — 適用済みハッシュ一致時は HAProxy の実状態を検査せず終了する**: `LastAppliedHash` が desired hash と一致すると、設定検証や systemd 状態確認をせず reconcile を返します（[main.go](../cmd/marmot-lb-agent/main.go#L91)）。HAProxy 停止や active config の手動変更を検知できるよう、プロセス稼働と active config の整合性も確認します。
- **低 — 外部コマンドに実行期限がない**: `haproxy -c` と `systemctl reload` を同期実行します（[main.go](../cmd/marmot-lb-agent/main.go#L104)、[main.go](../cmd/marmot-lb-agent/main.go#L119)）。コマンドが停止した場合に次の reconcile や停止処理へ進めないため、期限付き実行と中断時の結果記録を検討します。

## 取り組み順の提案

1. Volume の自動削除条件を明確化し、永続データが意図せず削除されないことを確認する。
2. Network のメンバー再調整を失敗時に再試行できるようにする。
3. mke-lb-controller の入力取得失敗時に最後の正常 HAProxy 設定を維持する。
4. Image / mke-node-controller のプロセス再起動をまたぐ処理状態を回復可能にする。
5. 各コントローラーの状態永続化エラーと reconcile 所要時間を共通に可観測化する。
