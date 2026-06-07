## ネットワーク・ロードバランサー機能の設計メモ

この文書は、marmot における L4 ロードバランサー機能を設計するためのメモである。既存の Gateway/VpnGateway 実装方針と整合するように、設計を進める。

### 1. 現状確認（2026-06-03 時点）

- OpenAPI v1 には kind: Gateway / kind: VpnGateway / kind: ApplicationLoadBalancer は定義済みだが、kind: NetworkLoadBalancer は未定義なので、今回 kind: NetworkLoadBalancer を新設する。
- ApplicationLoadBalancerは、HTTPレベルのL7ロードバランサーで、ヘルスチェックやセッション維持などの機能を有する。一方、NetworkLoadBalancerは、tcp/udp プロトコルを対象としたL4レベルのロードバランサーである。
- そのため本メモは「新規 API 追加前提の設計案」であり、実装着手時は API 定義追加と CLI 対応を同一フェーズで行う。
- 用語は既存資産と合わせる。
    - 外側ネットワーク: host-bridge（固定）
    - 内側ネットワーク: spec.internalVirtualNetwork
    - 許可元 初期実装は個人向けユースケースを想定し、spec.remoteCIDR（単数）とする。設定しなければ任意のアドレスからリクエストを受け入れる。

### 2. 目的とスコープ

- 目的
    - Public 側の 1 つの IP（VIP）で受けた通信を、内側仮想ネットワーク上の複数バックエンドへ分散する。

- スコープ内
    - VM ベース上の iptablesで負荷分散。HA-PROXYやNGINXを使用しない。
    - TCP/UDP レベル分散
    - 許可元 CIDR 制御
    - sessionPersistence は リクエスト元のIPアドレスでバックエンドサーバー限定で、既定値は false
    - backendSelector は labels のみを受け付ける
- スコープ外（初期リリース）
    - TLS 終端高度機能（証明書自動更新、SNI 多重運用）
    - WAF 機能
    - 自動スケール（HPA 相当）
    - IPv6 対応

### 3. API 設計案

```yaml
apiVersion: v1
kind: NetworkLoadBalancer
metadata:
    name: lb-1
spec:
    bindPublicIpAddress: 192.168.1.72
    internalVirtualNetwork: api-backends
    remoteCIDR: 192.168.1.0/24
    listeners:
        - name: tcp-service
          protocol: tcp
          vipPort: 4080
          backendPort: 3080
          backendSelector:
              matchLabels:
                  app: rest-api
          sessionPersistence:
              enabled: true
        - name: udp-service
          protocol: udp
          vipPort: 4200
          backendPort: 4100
          backendSelector:
              matchLabels:
                  app: udp-api
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
- spec.listeners[].protocol は TCP/UDP のみ許可。
- spec.listeners[].vipPort と spec.listeners[].backendPort は 1-65535。
- spec.listeners[].vipPort はオブジェクト内で重複禁止。
- spec.listeners[].backendSelector は matchLabels のみを許可し、名前直接指定はサポートしない。
- spec.listeners[].sessionPersistence.enabled の既定値は false。リクエスト元IPアドレスでバックエンドサーバーに対してのみ有効化できる。

### 4. データプレーン構成

- 1 NetworkLoadBalancer オブジェクトにつき 1 VM を作成。
- LB VM の配置先ノード選択は初期リリースでは仮想マシンのスケジューラーへ委任し、LB コントローラーの責務外とする。
- 初期実装では、LB VM の配置候補ホストはすべて同一のネットワーク/実行環境を持つ前提とする。
- NIC は 2 本。
    - 外側 NIC: host-bridge 固定 + bindPublicIpAddress 割当
    - 内側 NIC: internalVirtualNetwork に接続
- 外部公開ポートは Ansible で iptables 制御。

### 5. コントローラー仕様

#### 5.1 marmotd 側 NLB コントローラー（15 秒ループ）

- PENDING オブジェクトを検出し LB VM を作成。
- LB VM のスケジュール先は仮想マシンのスケジューラーに委任し、LB コントローラーは配置先を直接決定しない。
- VM 起動後、Ansible で初期ネットワーク設定を実施。
- 適用失敗時は CONFIGURING 状態で再試行し、上限回数超過で FAILED。
- apply リトライ上限とバックオフは環境変数で調整可能。
    - MARMOT_NLB_APPLY_RETRY_MAX（既定: 3）
    - MARMOT_NLB_APPLY_RETRY_BACKOFF_SECONDS（既定: 10）
- deletionTimestamp 設定後 15 秒経過で削除処理を開始。
- DELETING では iptables ルールの cleanup を実施し、cleanup 失敗時はオブジェクトを保持して再試行する。


### 6. 障害時動作

- Ansible 適用失敗
    - 3 回まで再試行、超過で status.status=FAILED と status.message を更新。

### 6.1 status.status の値

- PENDING: 作成直後、または初期構成が完了していない。
- ACTIVE: 構成反映済みで、サービス提供可能。
- FAILED: リトライ上限超過などで自動復旧を打ち切った。
- CONFIGURING: 設定反映中、または反映失敗後の再試行中。
- DELETING: 削除タイムスタンプが設定され、削除待ち

### 7. セキュリティ要件

- 許可元は spec.remoteCIDR のみ通過。
- デフォルトは全解放
- 管理用鍵は /etc/marmot/keys 配下を利用し、権限は最小化。
- playbook は deb 配布物に同梱し、postinst で再配置する。

### 8. 観測性要件

- status に最低限以下を持つ。
    - status.status: PENDING/CONFIGURING/ACTIVE/FAILED/DELETING
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
    - listener ごとに backendSelector を評価して独立にバックエンド集合を作る
    - listener ごとに vipPort/backendPort の対応で転送できる
    - sessionPersistence 有効時の固定化



