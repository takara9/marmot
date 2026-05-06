# Controller 層 OVS/VXLAN 統合実装計画

## 概要
方針 A（既存ステートマシン維持で段階的拡張）に基づいて、controller 層に OVS/VXLAN 操作責務を追加した実装を進める。  
PENDING/PROVISIONING/ACTIVE/DELETING の各段階に fabric 実体操作を組み込む。

## 実装済み内容

### 1. NetworkFabric インターフェース定義 (`pkg/networkfabric/interface.go`)
```
- EnsureBridge(): OVS ブリッジの作成・確認（冪等）
- EnsureVxlanMesh(): 指定ピアへの VXLAN トンネル群の作成・確認
- PruneVxlanMesh(): 不要なトンネルの削除（ノード離脱時など）
- DeleteBridge(): ブリッジとトンネル一式の削除
- GetBridgeStatus(): ブリッジ存在状態とピア数を返す
```

### 2. OVS 実装 (`pkg/networkfabric/ovs.go`)
- shell コマンド (`ovs-vsctl`) ベースの実装
- 既存あり前提の冪等処理
- エラーハンドリングと構造化ログ

### 3. API spec 拡張 (`api/marmot-api-v1.go`)
VirtualNetworkSpec に 4 フィールド追加（すべて optional）：
- `OverlayMode`: "none" (デフォルト) or "vxlan"
- `Vni`: VXLAN Network Identifier (0-16777215)
- `UnderlayInterface`: underlay 用インターフェース名
- `PeerPolicy`: "auto" or "manual"

### 4. DB status.message 標準化 (`pkg/db/db-virtual_network.go`)
- `UpdateVirtualNetworkStatusWithMessage()` メソッド追加
- ステータスコード別デフォルトメッセージ定義
- 段階名付きエラーメッセージ生成

### 5. Controller 統合 (`pkg/controller/network-controller.go`)

#### PROVISIONING フェーズ
1. `fabric.EnsureBridge()` で OVS ブリッジ作成
2. message: `fabric:ensure-bridge`
3. libvirt `DeployVirtualNetwork()` で ネットワーク定義・起動
4. message: `libvirt:define-start`
5. 成功時に ACTIVE へ遷移

#### DELETING フェーズ
**フォロワー:**
1. `ensureVirtualNetworkAbsent()` で libvirt destroy/undefine
2. `fabric.DeleteBridge()` でブリッジ削除
3. DB オブジェクト削除

**ヘッド:**
1. `DeleteVirtualNetwork()` で libvirt destroy/undefine
2. `fabric.DeleteBridge()` でブリッジ削除
3. DB オブジェクト削除

#### エラー時メッセージ
- `fabric:bridge-failed:<error>`
- `fabric:detach-failed:<error>`
- `libvirt:deploy-failed:<error>`
- `libvirt:delete-failed:<error>`

## 設計上の特徴

### 責務分離
- **controller**: オーケストレーション（何をするかの判定・順序制御）
- **networkfabric**: 実体操作（どのように操作するか）
- **virt**: libvirt 操作

### 冪等性
- すべての fabric 操作は冪等。already exists/not found は正常系に吸収

### 観測性
- status.message で進捗段階を明示
- 失敗点が特定可能

### スケーラビリティ
- 当面は full-mesh VXLAN（小規模向け）
- 将来 EVPN/ハブ型への拡張余地を設計

## 未実装・今後の課題

### 段階2: VXLAN ピア配布
- ホスト一覧からピア IP を抽出
- generation ベースの差分更新
- membership スナップショット決定

### 段階3: ACTIVE での drift 修復
- ノード再起動後の自己修復（ensure 再実行）

### 段階4: 2 段階削除
- DETACHED 状態を追加し、全ノード detach 完了待機
- ヘッドが最終 purge をコミット

### 段階5: テスト拡張
- OVS コマンド実行のモック化
- 冪等再実行検証
- 削除伝播確認

## テスト戦略

### 既存テスト
- `mactl-vm-deploy-2_test.go` の削除伝播テストは不足仕様を許容

### 新規テスト（推奨）
- controller ユニットテスト：fabric メソッド呼び出し検証
- 統合テスト：OVS 実体確認（libvirt 環境前提）

## 参考資料
- [削除伝播テスト競合メモ](network-delete-propagation-test-race.md)
- [ネットワーク設計メモ](MEMO-how-to-extend-virtual-network-over-node.md)
- [VLAN 設定例](MEMO-VLAN.md)
