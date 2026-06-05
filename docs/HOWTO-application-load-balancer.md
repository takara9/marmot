# Application LoadBalancer 運用マニュアル

このドキュメントは、Marmot の Application LoadBalancer (ALB) を作成・更新・削除・監視するための実運用手順です。
現行実装に基づき、未実装部分・制限事項も明記しています。

## 1. 概要

Application LoadBalancer は、内部ネットワーク上のサーバー群をラベル選択し、1つ以上の Listener で公開 IP へ振り分ける機能です。

主な特徴:
- 1 ALB リソースにつき 1 台の専用 VM を自動作成
- LB VM 上で HAProxy と marmot-lb-agent を使って設定を反映
- backendSelector.matchLabels に一致する稼働中サーバーを自動探索
- ステータス遷移で反映状況を管理 (PENDING/PROVISIONING/CONFIGURING/ACTIVE/DEGRADED/FAILED/DELETING)

対応リソース名:
- applicationloadbalancer
- application-load-balancer
- alb (短縮名)

## 2. 前提条件

- Marmot がインストール済みで、marmot サービスが稼働していること
- host-bridge と内部 VirtualNetwork が事前に作成済みであること
- 内部 VirtualNetwork 名は予約ネットワーク(default, host-bridge, ovs-network)ではないこと
- LB VM へ SSH 接続できる鍵が利用可能であること
  - Controller は秘密鍵を使って ansible-playbook 実行と状態取得を行います
- backend として使う Server に適切な metadata.labels と内部 IP が設定されていること

## 3. 最小マニフェスト例

以下は HTTP Listener 1 本の最小構成例です。

```yaml
apiVersion: v1
kind: ApplicationLoadBalancer
metadata:
  name: web-alb
spec:
  bindPublicIpAddress: 192.168.10.80
  internalVirtualNetwork: app-net
  listeners:
    - name: http
      protocol: HTTP
      vipPort: 80
      backendPort: 8080
      backendSelector:
        matchLabels:
          app: web
```

作成:

```bash
mactl create -f alb.yaml
# または
mactl create applicationloadbalancer -f alb.yaml
# または
mactl create alb -f alb.yaml
```

確認:

```bash
mactl get alb
mactl describe alb web-alb
```

## 4. Listener 設定

Listener ごとの主なパラメータ:
- name: Listener 名 (同一 ALB 内で一意)
- protocol: HTTP / TCP / UDP
- vipPort: 公開ポート
- backendPort: バックエンド接続先ポート
- backendSelector.matchLabels: バックエンド選択条件 (必須)
- loadBalancingAlgorithm: roundrobin または random (省略時 roundrobin)
- healthCheck.enabled: true の場合、HTTP Listener のみ有効
- sessionPersistence.enabled: true の場合、HTTP Listener のみ有効

補足:
- Listener 名の重複はエラー
- vipPort の重複はエラー
- backendPort, vipPort は 1-65535

## 5. ステータス遷移

代表的な遷移:
- PENDING: 作成直後。LB VM 作成の準備
- PROVISIONING: LB VM のプロビジョニング中
- CONFIGURING: Ansible/Agent による設定反映中
- ACTIVE: 設定適用済み
- DEGRADED: 一部機能劣化 (バックエンド欠落、エージェント状態読取失敗など)
- FAILED: 致命的エラー
- DELETING: 削除処理中

確認コマンド:

```bash
mactl get alb
mactl describe alb web-alb
```

トラブル解析時は describe の Message を最優先で確認してください。

## 6. 更新・削除

### 更新

apply で更新できます。

```bash
mactl apply -f alb.yaml
```

更新時の制約:
- spec.bindPublicIpAddress は immutable (更新不可)
- spec.internalVirtualNetwork は immutable (更新不可)
- listeners と remoteCIDR は更新可能

### 削除

```bash
mactl delete alb web-alb
```

削除は非同期です。DELETING を経由して完了します。

## 7. 既知の制限・未実装

この章は現行実装ベースの制限事項です。

1) remoteCIDR は実効制御に未反映
- spec.remoteCIDR は受理・保存されますが、現行の HAProxy/iptables 反映には使われません。
- つまり ALB への到達元制限としては、現時点では機能しません。

2) UDP は専用実装が未完了
- API 上は protocol: UDP を受理します。
- ただし設定生成では HTTP 以外を tcp モードとして扱うため、UDP 固有の L4 動作を保証しません。
- UDP 要件がある場合は現時点では非推奨です。

3) HealthCheck の詳細パラメータ未使用
- healthCheck.intervalSeconds / timeoutSeconds / unhealthyThreshold は現行設定生成で未使用です。
- 実際に反映されるのは HTTP の Path を使った httpchk 相当のみです。

4) LB VM スペックは固定
- 自動作成される LB VM は固定スペック (CPU 1, Memory 2048MB, OS ubuntu24.04) です。
- ALB マニフェストから VM スペックを直接調整する仕組みはありません。

5) 名称一意性は internalVirtualNetwork スコープ
- 同一 internalVirtualNetwork 内では metadata.name 重複不可
- bindPublicIpAddress は全 ALB で一意

6) バックエンド選択は「稼働中サーバー + ラベル一致 + 内部ネットワークIP」
- 条件を満たす backend が 0 件の Listener があると DEGRADED になります。

## 8. 運用時の注意

- metadata.labels の managedBy など内部管理ラベルは controller が利用するため、手動上書きしないでください。
- LB VM や関連 Server を直接削除した場合、ALB 側が再作成または削除される場合があります。
- ansible-playbook 実行失敗が継続すると FAILED へ遷移します。
- Agent 状態ファイル取得失敗が継続すると DEGRADED へ遷移します。

## 9. トラブルシュート

### 例1: 作成直後に FAILED

確認ポイント:
- internalVirtualNetwork が存在するか
- 予約ネットワーク名を使っていないか
- bindPublicIpAddress が他 ALB と重複していないか
- controller ノードから LB VM へ SSH 可能か

### 例2: DEGRADED から復帰しない

確認ポイント:
- Listener の backendSelector.matchLabels に合致する RUNNING サーバーがあるか
- 対象サーバーが internalVirtualNetwork 上に IP を持っているか
- LB VM 上の marmot-lb-agent が稼働しているか

### 例3: apply で更新できない

確認ポイント:
- bindPublicIpAddress / internalVirtualNetwork を変更していないか
- immutable fields changed エラーが出た場合は、新規作成 + 切替で対応

## 10. 参考コマンド

```bash
# 一覧
mactl get alb

# 詳細
mactl describe alb <name>

# JSON/YAML 出力
mactl get alb -o json
mactl get alb -o yaml

# ラベル絞り込み
mactl get alb -l env=prod
```
