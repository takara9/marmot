## ロードバランサー機能の設計メモ v2

この文書は、marmot における L4/L7 ロードバランサー機能を設計するためのメモである。既存の Gateway/VpnGateway 実装方針と整合するように、設計を進める。

### 1. 現状確認（2026-06-03 時点）

- OpenAPI v1 には kind: Gateway / kind: VpnGateway は定義済みだが、kind: LoadBalancer は未定義なので、今回 kind: LoadBalancer を新設する。
- そのため本メモは「新規 API 追加前提の設計案」であり、実装着手時は API 定義追加と CLI 対応を同一フェーズで行う。
- 用語は既存資産と合わせる。
    - 外側ネットワーク: host-bridge（固定）
    - 内側ネットワーク: spec.internalVirtualNetwork
    - 許可元 互換目的で spec.remoteCIDR（単数）とする。設定しなければ任意のアドレスからリクエストを受け入れる。

### 2. 目的とスコープ

- 目的
    - Public 側の 1 つの IP（VIP）で受けた通信を、内側仮想ネットワーク上の複数バックエンドへ分散する。
    - バックエンド増減に追従して自動的に HAProxy 設定を反映する。
- スコープ内
    - VM ベースの LB（HAProxy 同梱）
    - TCP レベル分散（HTTP/HTTPS を含む）
    - 許可元 CIDR 制御、ヘルスチェック、セッション維持（cookie）
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
    serverPorts:
        - http
        - https
    remoteCIDR: 192.168.1.0/24
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
        # cookieName は省略可。省略時は metadata.name から自動決定
```

#### 3.1 必須項目

- spec.bindPublicIpAddress
- spec.internalVirtualNetwork
- spec.serverPorts

#### 3.2 更新可否（想定）

- 更新禁止（immutable）
    - spec.bindPublicIpAddress
    - spec.internalVirtualNetwork
- 更新許可
    - spec.serverPorts
    - spec.remoteCIDRs
    - spec.backendSelector
    - spec.healthCheck
    - spec.sessionPersistence

#### 3.3 バリデーション

- metadata.name は spec.internalVirtualNetwork 単位で一意。
- spec.bindPublicIpAddress はクラスタ全体で一意。
- spec.bindPublicIpAddress は有効な IP 文字列。
- spec.bindPublicIpAddress は初期リリースでは IPv4 のみ許可。
- spec.remoteCIDRs は CIDR 文字列配列。空は許可（0.0.0.0/0 相当）。
- spec.backendSelector は matchLabels のみを許可し、名前直接指定はサポートしない。
- spec.loadBalancingAlgorithm は random または roundrobin のみ許可し、未指定時は roundrobin とする。
- spec.serverPorts は以下のみ許可。
    - サービス名（http, https, ssh など）
    - 数値形式 n/tcp, n/udp
- サービス名解決の既定プロトコルは tcp。
- spec.sessionPersistence.cookieName は省略可。未指定時は metadata.name から HAProxy で利用可能な cookie 名へ正規化して自動設定する。
- 自動設定時は英数字と _ のみを使用し、先頭は英字とする。metadata.name を正規化した結果をそのまま使い、同一オブジェクト更新時も維持する。

### 4. データプレーン構成

- 1 LoadBalancer オブジェクトにつき 1 VM を作成。
- NIC は 2 本。
    - 外側 NIC: host-bridge 固定 + bindPublicIpAddress 割当
    - 内側 NIC: internalVirtualNetwork に接続
- LB VM 内で HAProxy と LB コントローラーを稼働。
- 外部公開ポートは Ansible で iptables 制御。

### 5. コントローラー仕様

#### 5.1 marmotd 側 LB コントローラー（15 秒ループ）

- PENDING オブジェクトを検出し LB VM を作成。
- VM 起動後、Ansible で初期ネットワーク設定を実施。
- 失敗時はリトライし、3 回超過で FAILED。
- deletionTimestamp 設定後 15 秒経過で削除処理を開始。

#### 5.2 LB VM 内 HAProxy コントローラー（5 秒ループ）

- marmotd API を定期監視し、対象バックエンド集合の差分を検出。
- 差分ありの場合に haproxy.cfg を再生成し、構文検証後に reload。
- ロードバランシングアルゴリズムは spec で random または roundrobin を選択でき、未指定時は roundrobin とする。
- 健全性判定
    - /healthz が 404 の場合: HTTP パスチェックを無効化し TCP チェックへフォールバック。
    - タイムアウト/5xx などが unhealthyThreshold 回継続: そのバックエンドを一時除外。
- sessionPersistence.enabled=true かつ cookieName 未指定の場合は metadata.name 由来の安定した cookie 名を使う。更新時も同一オブジェクトでは cookie 名を維持する。
- バックエンド VM の再起動は LB コントローラーの責務外（分離原則）。必要なら上位 controller に event 通知。

### 6. 障害時動作

- Ansible 適用失敗
    - 3 回まで再試行、超過で status.status=FAILED と status.message を更新。
- HAProxy reload 失敗
    - 旧設定を維持し status.message に失敗理由を記録。
- 監視 API 通信失敗
    - 一定回数までは現行設定維持、閾値超過で Degraded 相当メッセージを記録。

### 6.1 status.status の値

- PENDING: 作成直後、または初期構成が完了していない。
- ACTIVE: 構成反映済みで、サービス提供可能。
- DEGRADED: オブジェクト自体は維持するが、一部 backend 除外や API 通信断などで期待状態を満たしていない。
- FAILED: リトライ上限超過などで自動復旧を打ち切った。

### 7. セキュリティ要件

- 許可元は spec.remoteCIDRs のみ通過。
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
    - remoteCIDRs の CIDR 妥当性
    - immutable 項目変更拒否
- 一意制約
    - 同一 internalVirtualNetwork で同名拒否
    - bindPublicIpAddress のクラスタ一意性
- コントローラー
    - VM 作成成功で ACTIVE 遷移
    - Ansible 3 回失敗で FAILED
    - delete 時の後始末完了
- LB 動作
    - backend 追加/削除で再設定反映
    - /healthz 404 時に TCP チェックへフォールバック
    - sessionPersistence 有効時の固定化

### 10. 段階的リリース案

- Phase 1: API/DB/CLI（保存・取得・表示）
- Phase 2: marmotd コントローラー（VM 作成/削除）
- Phase 3: LB VM 内コントローラー + HAProxy 自動反映
- Phase 4: deb 同梱、postinst 反映、E2E テスト

