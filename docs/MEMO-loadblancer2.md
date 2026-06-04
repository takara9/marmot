## ロードバランサー機能の設計メモ v2

この文書は、marmot における L4/L7 ロードバランサー機能を設計するためのメモである。既存の Gateway/VpnGateway 実装方針と整合するように、設計を進める。

### 1. 現状確認（2026-06-03 時点）

- OpenAPI v1 には kind: Gateway / kind: VpnGateway は定義済みだが、kind: LoadBalancer は未定義なので、今回 kind: LoadBalancer を新設する。
- そのため本メモは「新規 API 追加前提の設計案」であり、実装着手時は API 定義追加と CLI 対応を同一フェーズで行う。
- 用語は既存資産と合わせる。
    - 外側ネットワーク: host-bridge（固定）
    - 内側ネットワーク: spec.internalVirtualNetwork
    - 許可元 初期実装は個人向けユースケースを想定し、spec.remoteCIDR（単数）とする。設定しなければ任意のアドレスからリクエストを受け入れる。

### 2. 目的とスコープ

- 目的
    - Public 側の 1 つの IP（VIP）で受けた通信を、内側仮想ネットワーク上の複数バックエンドへ分散する。
    - バックエンド増減に追従して自動的に HAProxy 設定を反映する。
- スコープ内
    - VM ベースの LB（HAProxy 同梱）
    - TCP レベル分散（HTTP/HTTPS を含む）
    - 許可元 CIDR 制御
    - healthCheck は HTTP プロトコルを使用するバックエンドサーバー限定
    - sessionPersistence は HTTP プロトコルを使用するバックエンドサーバー限定で、既定値は false
    - リクエスト分散のアルゴリズムは、ランダムとラウンドロビンの２種類
    - backendSelector は labels のみを受け付ける
- スコープ外（初期リリース）
    - TLS 終端高度機能（証明書自動更新、SNI 多重運用）
    - WAF 機能
    - 自動スケール（HPA 相当）
    - IPv6 対応

### 3. API 設計案

```yaml
apiVersion: v1
kind: LoadBalancer
metadata:
    name: lb-1
spec:
    bindPublicIpAddress: 192.168.1.70
    internalVirtualNetwork: web-backends
    remoteCIDR: 192.168.1.0/24
    listeners:
        - name: web-http
          protocol: HTTP
          vipPort: 80
          backendPort: 8080
          backendSelector:
              matchLabels:
                  app: web
          loadBalancingAlgorithm: roundrobin
          healthCheck:
              enabled: true
              path: /healthz
              intervalSeconds: 5
              timeoutSeconds: 2
              unhealthyThreshold: 10
          sessionPersistence:
              enabled: true
              cookieName: WEB80
        - name: web-admin
          protocol: HTTP
          vipPort: 8081
          backendPort: 18081
          backendSelector:
              matchLabels:
                  app: web-admin
          loadBalancingAlgorithm: random
          healthCheck:
              enabled: true
              path: /admin/healthz
              intervalSeconds: 10
              timeoutSeconds: 2
              unhealthyThreshold: 3
          sessionPersistence:
              enabled: false
        - name: tls-pass
          protocol: TCP
          vipPort: 443
          backendPort: 8443
          backendSelector:
              matchLabels:
                  app: web-tls
          loadBalancingAlgorithm: roundrobin
          healthCheck:
              enabled: false
          sessionPersistence:
              enabled: false
```

#### 3.1 必須項目

- spec.bindPublicIpAddress
- spec.internalVirtualNetwork
- spec.listeners

#### 3.2 更新可否（想定）

- 更新禁止（immutable）
    - spec.bindPublicIpAddress
    - spec.internalVirtualNetwork
- 更新許可
    - spec.listeners
    - spec.remoteCIDR

#### 3.3 バリデーション

- metadata.name は spec.internalVirtualNetwork 単位で一意。
- spec.bindPublicIpAddress はクラスタ全体で一意。
- spec.bindPublicIpAddress は有効な IP 文字列。
- spec.bindPublicIpAddress は初期リリースでは IPv4 のみ許可。
- spec.remoteCIDR は単一の CIDR 文字列。空は許可（0.0.0.0/0 相当）。
- spec.remoteCIDR を単数とするのは意図的な制約で、初期実装では個人向け利用を想定する。
- spec.listeners は 1 件以上必須。
- spec.listeners[].name はオブジェクト内で一意。
- spec.listeners[].protocol は HTTP/TCP/UDP のみ許可。
- spec.listeners[].vipPort と spec.listeners[].backendPort は 1-65535。
- spec.listeners[].vipPort はオブジェクト内で重複禁止。
- spec.listeners[].backendSelector は matchLabels のみを許可し、名前直接指定はサポートしない。
- spec.listeners[].loadBalancingAlgorithm は random または roundrobin のみ許可し、未指定時は roundrobin とする。
- spec.listeners[].healthCheck は HTTP プロトコルを使用するバックエンドサーバーに対してのみ有効化できる。HTTP 以外では enabled=false 固定とする。
- spec.listeners[].sessionPersistence.enabled の既定値は false。HTTP プロトコルを使用するバックエンドサーバーに対してのみ有効化できる。
- spec.listeners[].sessionPersistence.cookieName は HTTP プロトコルを使用するバックエンドサーバーで sessionPersistence.enabled=true の場合のみ省略可。未指定時は metadata.name から HAProxy で利用可能な cookie 名へ正規化して自動設定する。
- 自動設定時は英数字と _ のみを使用し、先頭は英字とする。metadata.name を正規化した結果をそのまま使い、同一オブジェクト更新時も維持する。

### 4. データプレーン構成

- 1 LoadBalancer オブジェクトにつき 1 VM を作成。
- LB VM の配置先ノード選択は初期リリースでは仮想マシンのスケジューラーへ委任し、LB コントローラーの責務外とする。
- 初期実装では、LB VM の配置候補ホストはすべて同一のネットワーク/実行環境を持つ前提とする。
- NIC は 2 本。
    - 外側 NIC: host-bridge 固定 + bindPublicIpAddress 割当
    - 内側 NIC: internalVirtualNetwork に接続
- LB VM 内で HAProxy と LB コントローラーを稼働。
- 外部公開ポートは Ansible で iptables 制御。

### 5. コントローラー仕様

#### 5.1 marmotd 側 LB コントローラー（15 秒ループ）

- PENDING オブジェクトを検出し LB VM を作成。
- LB VM のスケジュール先は仮想マシンのスケジューラーに委任し、LB コントローラーは配置先を直接決定しない。
- VM 起動後、Ansible で初期ネットワーク設定を実施。
- 失敗時はリトライし、3 回超過で FAILED。
- deletionTimestamp 設定後 15 秒経過で削除処理を開始。

#### 5.2 LB VM 内 HAProxy コントローラー（5 秒ループ）

- marmotd API を定期監視し、対象バックエンド集合の差分を検出。
- 差分ありの場合に haproxy.cfg を再生成し、構文検証後に reload。
- listeners ごとに frontend/backend を生成し、vipPort と backendPort の差異を反映する。
- listeners ごとに backendSelector を評価し、対象バックエンド集合を独立に決定する。
- ロードバランシングアルゴリズムは listener ごとに random または roundrobin を選択でき、未指定時は roundrobin とする。
- healthCheck は HTTP プロトコルを使用するバックエンドサーバーに対してのみ listener 単位で実施する。
- 健全性判定
    - /healthz の HTTP 応答が unhealthyThreshold 回連続で失敗した backend は一時除外する。
    - healthCheck 失敗が発生した場合、LoadBalancer の status.status は DEGRADED とする。
- sessionPersistence は HTTP プロトコルを使用するバックエンドサーバーに対してのみ listener 単位で有効化できる。enabled=true かつ cookieName 未指定の場合は metadata.name 由来の安定した cookie 名を使う。更新時も同一オブジェクトでは cookie 名を維持する。
- バックエンド VM の再起動は LB コントローラーの責務外（分離原則）。必要なら上位 controller に event 通知。

#### 5.3 LB VM 内 HAProxy コントローラーの実装方式

- LB VM 内に専用エージェント（例: marmot-lb-agent）を常駐させる。
- エージェントは systemd 管理で自動起動・自動再起動する。
- 通信経路は段階的に運用する。
    - 初期実装: HAProxy コントローラーと marmotd API の通信は host-bridge 経由とする。
    - 初期実装: HAProxy コントローラーと marmotd API 間は TLS と認証を導入しない（同一環境前提）。
    - 発展型: 制御通信は専用制御プレーン（管理ネットワーク）へ移行し、host-bridge はデータプレーン用途を優先する。
    - 発展型: TLS と認証を導入し、制御通信を強化する。
- 5 秒ごとに以下を実行する。
    - 対象 LoadBalancer と listener ごとの backendSelector に一致するバックエンド情報を API から取得。
    - listeners 単位で desired な HAProxy 設定を生成。
    - 前回適用済みの設定ハッシュと比較し、差分がある場合のみ反映処理を実行。
- marmotd 側は staged/applied を分離して管理する。
    - stagedConfigHash: LB VM へ配備済みの desired 設定ハッシュ。
    - appliedConfigHash: LB VM 内 agent が適用完了した設定ハッシュ。
    - stagedConfigAt: stagedConfigHash を配備した時刻。
- ACTIVE 遷移は hash 一致のみで確定しない。
    - agent state の lastAppliedHash が desired と一致すること。
    - agent state の lastAppliedAt が stagedConfigAt 以上であること。
    - 上記を満たした場合のみ appliedConfigHash を更新し、ACTIVE とする。
- 反映処理は安全側に倒す。
    - 一時ファイルに haproxy.cfg を生成。
    - `haproxy -c -f <tempfile>` で構文検証。
    - 検証成功時のみ本番設定へ置換し、`systemctl reload haproxy` を実行。
    - 検証失敗または reload 失敗時は旧設定を維持し、status.message に失敗理由を記録。
- 同時反映を防ぐため、単一実行ロック（ファイルロック等）を導入する。
- marmotd 側は SSH で LB VM 内 state file を参照し、agent の適用結果を監視する。
- API 一時障害/agent state 読み取り障害時は最後の有効設定を維持し、しきい値超過で status.status を DEGRADED に更新する。
    - 現在実装のしきい値: 連続読み取り失敗 3 回で DEGRADED。
    - 読み取り成功時は失敗カウンタをリセットする。
- DEGRADED からの復帰時はフラップ防止のため連続成功しきい値を適用する。
    - 現在実装のしきい値: 連続読み取り成功 2 回で ACTIVE 復帰。
- marmotd 側のしきい値・間隔は環境変数で上書きできる。
    - MARMOT_LB_CONTROLLER_INTERVAL_SECONDS: LB コントローラーの制御ループ間隔（秒）。
    - MARMOT_LB_AGENT_STATE_READ_MAX_FAILURES: 読み取り失敗で DEGRADED に遷移する連続失敗回数。
    - MARMOT_LB_AGENT_RECOVERY_SUCCESS_REQUIRED: DEGRADED から ACTIVE へ戻す連続成功回数。
    - いずれも未設定または不正値（0 以下/非数値）は既定値を使用する。
- healthCheck 復旧時または失敗バックエンド消滅時は status.status を ACTIVE に戻す。
- status.status の最終責任者は段階で切り替える。
    - HAProxy コントローラー起動前: marmotd が status.status を管理する。
    - HAProxy コントローラー起動後: HAProxy コントローラーが status.status を管理する。

### 6. 障害時動作

- Ansible 適用失敗
    - 3 回まで再試行、超過で status.status=FAILED と status.message を更新。
- HAProxy reload 失敗
    - 旧設定を維持し status.message に失敗理由を記録。
- healthCheck 失敗
    - unhealthyThreshold 回連続で失敗した backend を一時除外し、status.status=DEGRADED を記録。
- 監視 API 通信失敗
    - 一定回数までは現行設定維持、閾値超過で status.status=DEGRADED とメッセージを記録。
    - 現在実装では agent state 読み取り失敗を 3 回まで許容し、4 回目ではなく 3 回目到達時点で DEGRADED とする。
    - 閾値は MARMOT_LB_AGENT_STATE_READ_MAX_FAILURES で調整可能。

### 6.1 status.status の値

- PENDING: 作成直後、または初期構成が完了していない。
- ACTIVE: 構成反映済みで、サービス提供可能。
- DEGRADED: オブジェクト自体は維持するが、healthCheck 失敗による backend 除外や API 通信断などで期待状態を満たしていない。
- FAILED: リトライ上限超過などで自動復旧を打ち切った。

### 6.2 DEGRADED からの復帰条件

- healthCheck 対象バックエンドから正常応答が返り、失敗状態が解消されたとき。
- backendSelector でマッチしたバックエンド集合の中に、healthCheck 失敗中のバックエンドが存在しなくなったとき。
- 上記を満たし、かつ agent state 読み取りが連続成功しきい値を満たしたタイミングで status.status を ACTIVE に戻す。
- 連続成功しきい値は MARMOT_LB_AGENT_RECOVERY_SUCCESS_REQUIRED で調整可能。

### 7. セキュリティ要件

- 許可元は spec.remoteCIDR のみ通過。
- デフォルト拒否（明示ポートのみ開放）。
- 管理用鍵は /etc/marmot/keys 配下を利用し、権限は最小化。
- playbook は deb 配布物に同梱し、postinst で再配置する。

### 8. 観測性要件

- status に最低限以下を持つ。
    - status.status: PENDING/ACTIVE/DEGRADED/FAILED
    - status.message: 最後の失敗要約
    - status.lastUpdated: 最終更新時刻
- ログ
    - 反映前後の backend 数
    - 反映対象ポート
    - reload 成否
- メトリクス（将来拡張）
    - backend up/down 数
    - reload 回数・失敗回数

### 9. テスト観点（最低限）

- API バリデーション
    - bindPublicIpAddress 必須・IP 妥当性
    - remoteCIDR の CIDR 妥当性
    - listeners が 1 件以上であること
    - listener ごとに backendSelector を指定すること
    - listener の vipPort 重複禁止
    - healthCheck は HTTP プロトコルを使用するバックエンドサーバー時のみ有効化可能
    - sessionPersistence は HTTP プロトコルを使用するバックエンドサーバー時のみ有効化可能かつ既定値 false
    - immutable 項目変更拒否
- 一意制約
    - 同一 internalVirtualNetwork で同名拒否
    - bindPublicIpAddress のクラスタ一意性
- コントローラー
    - VM 作成成功で ACTIVE 遷移
    - Ansible 3 回失敗で FAILED
    - agent state 読み取り失敗が連続 3 回で DEGRADED
    - DEGRADED 復帰時に連続成功しきい値（2 回）を満たすまで ACTIVE に戻さない
    - 上記しきい値と制御間隔は環境変数で上書き可能
    - delete 時の後始末完了
- LB 動作
    - backend 追加/削除で再設定反映
    - listener ごとに backendSelector を評価して独立にバックエンド集合を作る
    - listener ごとに vipPort/backendPort の対応で転送できる
    - HTTP healthCheck 失敗時に backend を除外し status.status が DEGRADED になる
    - healthCheck 回復または失敗バックエンド消滅時に status.status が ACTIVE へ復帰する
    - sessionPersistence 有効時の固定化

### 10. 注意点

- healthCheck の path 指定と cookie ベースの sessionPersistence は HTTP モード前提。TCP/UDP listener では無効化（enabled=false）する。
- HTTPS で cookie ベース sessionPersistence を使う場合は LB 側で TLS 終端が必要。TLS passthrough の場合は source IP など別方式を使う。
- 実装は listener 単位で HAProxy 設定へ変換する。1 listener が 1 frontend/1 backend の基本単位になる。

### 11. 段階的リリース案

- Phase 1: API/DB/CLI（保存・取得・表示）
- Phase 2: marmotd コントローラー（VM 作成/削除）
- Phase 3: LB VM 内コントローラー + HAProxy 自動反映
- Phase 3 補足（実装済み）: staged/applied handoff、適用時刻ゲート、読み取り失敗/復帰しきい値、環境変数によるしきい値上書き
- Phase 4: deb 同梱、postinst 反映、E2E テスト

