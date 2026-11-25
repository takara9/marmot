# MEMO: LVM データボリュームの定期スナップショット対応の検討

LVM データボリューム（`type: lvm` / `kind: data`）で定期的にスナップショットを取得できるようにするために必要な変更の整理メモ。

## 現状

- `lvm.CreateSnapshot`（[pkg/lvm/lvm.go](../pkg/lvm/lvm.go)）は存在するが、呼び出しは
  [pkg/marmotd/volumes.go](../pkg/marmotd/volumes.go) の **OS ブートボリューム生成時のみ**
  （OS イメージテンプレート LV からの CoW スナップショットによる高速クローン。CoW 領域サイズは 4GB 固定）
- データボリュームは素の LV 作成のみで、スナップショット機能・API・スケジューラは一切なし
- 定期実行の仕組みとしては `startRevokedAPIKeyCleanupWorker`
  （[pkg/marmotd/api-core.go](../pkg/marmotd/api-core.go)）のような ticker ベースのワーカーパターンが既にあり、流用できる
- QCOW2 用のスナップショットヘルパー（`CreateSnapshotQcow` 等、[pkg/qcow/qcow.go](../pkg/qcow/qcow.go)）は
  存在するがテスト以外から未使用で、API にも接続されていない

## 必要な変更（6 領域）

### 1. API 定義（api/marmot-api-v1.yaml → コード生成）

- `VolSpec` に **スナップショットポリシー** を追加
  - 例: `snapshotPolicy: { enabled, intervalMinutes (または cron 式), retentionCount, snapshotSizeGB }`
- スナップショットを一覧・手動取得・削除・リストアできる **エンドポイント追加**
  - `/volume/{volumeId}/snapshots` (GET/POST)
  - `/volume/{volumeId}/snapshots/{snapshotId}` (GET/DELETE)
  - 必要なら `/restore`
- 規約どおり `api/marmot-api-v1.go` は直接編集せず、YAML 変更 → コード生成で反映する

### 2. etcd データモデル（pkg/db）

- スナップショット記録用のキー空間を追加（例: `/snapshot/{volumeId}/{snapshotId}`）
  - 保持情報: 親ボリューム ID、LV 名、作成時刻、サイズ、状態
- 世代管理（retention 超過分の特定）に使う一覧取得関数

### 3. lvm パッケージ（pkg/lvm）

- `CreateSnapshot` はそのまま流用可能
- 追加が必要なもの:
  - スナップショット一覧取得（タグ `snapshot`, `marmot` でのフィルタ）
  - 削除（`RemoveLV` で代用可能だが、スナップショット専用の安全チェック付きラッパが望ましい）
  - リストアする場合は `lvconvert --merge` 相当の実装（go-lvm が未対応なら `exec` 呼び出し）

### 4. 定期実行ワーカー（pkg/marmotd）

- `startRevokedAPIKeyCleanupWorker` と同型の **スナップショットスケジューラ** を新設
  - ticker で全ボリュームを走査 → ポリシー有効かつ期限到来した LVM データボリュームにスナップショット取得
  - retention 超過分の古いスナップショットを削除
  - 取得前に `CheckVG` で VG 空き容量を確認し、不足時はスキップ＋状態記録

### 5. ノード配置の考慮

- LV は特定ハイパーバイザーノード上にあるため、**スナップショット取得はその LV を持つノードで実行**する必要がある
- 各ノードのワーカーが「自ノードのボリュームのみ」を対象にする設計が必要
  （`resolveImageTemplateByVolumeNode` と同様のノード解決ロジック）

### 6. mactl CLI（cmd/mactl）

- `mactl snapshot list/create/delete` 相当のサブコマンド、または
  マニフェストの `snapshotPolicy` を apply で設定できるようにする

## 設計上の留意点

1. **CoW サイズ枯渇**: 従来型 LVM スナップショットは CoW 領域が溢れると invalid になる。
   定期スナップショットを複数世代保持する用途なら、**thin pool（thin-provisioning）への移行を検討する価値が高い**
   （thin snapshot は容量固定不要・低オーバーヘッド）。ただし変更規模が大きく、別判断となる
2. **整合性**: VM 稼働中の取得は crash-consistent。アプリ整合性が必要なら
   qemu-guest-agent 経由の `fsfreeze` 連携が将来課題（初期スコープ外推奨）
3. **iSCSI データボリューム**: `configureISCSIForVolume` で export 中の LV も対象になり得るため、
   スナップショット自体は可能だが、リストア時の扱い（イニシエータ切断）に注意が必要

## 実装前に決めること

- リストア機能を初期スコープに含めるか
- スケジュール指定は interval 方式で足りるか（cron 式が必要か）
