# LoadBalancer v2 再設計メモ（実運用前提）

## 目的

現行の内部限定 OVN LB 実装は、`status=ACTIVE` と実到達性が一致しないケースがある。
この v2 では、コントロールプレーンとデータプレーンを一体で設計し、
「作成できた」ではなく「到達できる」を成立条件にする。

## 非目標

- L7 機能（WAF, URL ルーティング, TLS 終端高度機能）
- 複雑な重み付けポリシー
- マルチクラスタ統合

## v2 の成立条件（Done Definition）

- 同一 internalVirtualNetwork 上の任意ノード配置で VIP 到達が成功する
- `ACTIVE` は疎通検証パスを通過した場合のみ遷移する
- ノード再起動、leader 交代、LB 更新後に自動再収束する
- stale OVN オブジェクトが残っても reconcile が自己修復する

## 方式

### 1. トポロジ原則

- 内部 LB は OVN native L4 を継続利用
- 論理スイッチ名は `internalVirtualNetwork` 名を一次キーに統一
- `VirtualNetworkID` は識別タグ用途（external_ids）に限定
- 同名ネットワークはクラスタ内で必ず同一 logical switch へ収束

### 2. VIP 到達性

- VIP の L2 到達（ARP/ND）と L4 転送（LB）を分離して管理する
- ARP 成立だけで通信を吸い込む localport ブラックホールを禁止
- VIP 到達性は ovn-trace と実curlで検証し、未成立時は `PROVISIONING` 維持

### 3. 状態遷移（厳格化）

- `PENDING`:
  - spec バリデーション
  - backend 解決
  - OVN desired 作成
- `PROVISIONING`:
  - OVN 反映
  - 到達性検証（ARP/ND + TCP）
  - 成功時のみ `ACTIVE`
- `ACTIVE`:
  - drift 検知（spec, backend, logical switch, OVN実体差分）
  - 差分ありで `PROVISIONING` へ戻す
- `FAILED`:
  - 利用者入力不正のみ
  - 一時的な環境不整合は `PROVISIONING` で再試行

## データモデル方針

- labels は観測値を保持
  - `logicalSwitchName`
  - `ovnLoadBalancerName`
  - `resolvedVirtualIpAddress`
  - `resolvedBackends`
  - `appliedConfigHash`
- `appliedConfigHash` に以下を必須含有
  - internalVirtualNetwork 名
  - logicalSwitchName
  - VIP
  - ports
  - backends

## Reconcile 仕様（最小）

1. `GetVirtualNetworkByName` で対象ネットワーク解決
2. `logicalSwitchName` 一意解決
3. LB object ensure/update
4. LS への attach ensure
5. stale object cleanup（同VIP競合, 旧ID残骸）
6. 到達性検証（軽量）
7. `ACTIVE` 反映

## 運用安全策

- 機能フラグ `MARMOT_LB_V2_ENABLED` を導入
  - デフォルト `false`
  - CI と検証環境で段階的に `true`
- 失敗時の自動ロールバック手段を用意
  - LB object detach
  - stale VIP object purge

## テストゲート

### Unit

- logical switch 名解決（name 優先）
- hash 変化点（LS名変更, backend変更, VIP変更）
- stale cleanup（旧ID競合）

### Integration

- 3ノード分散配置で web-client -> VIP 応答確認
- leader 再選出後の LB 到達性維持
- LB delete/recreate の競合再現と自己修復

### E2E 合否

- backend直アクセス成功
- VIPアクセス成功
- `status=ACTIVE` と実疎通が一致

## 実装順序（推奨）

1. 状態遷移と `ACTIVE` 判定の厳格化
2. naming/key 一貫化（network名主キー）
3. stale cleanup の強化
4. 到達性検証フック導入
5. フラグ運用で段階リリース

## リリース方針

- v2 は実験フラグ付きで出荷
- 検証結果が揃うまで現行の安定経路を既定値に維持
- 合格後に既定値を v2 に切替
