package networkfabric

import (
	"github.com/takara9/marmot/api"
)

// NetworkFabric は OVS などのオーバーレイネットワーク実体操作を担当する。
// controller 層がこのインターフェースを通じて、libvirt とは独立に overlay を管理する。
type NetworkFabric interface {
	// EnsureBridge はローカルノードで OVS ブリッジを作成・確認する。
	// 既存あり前提で冪等に実行される。
	// vnet: 対象ネットワークオブジェクト
	// 戻り値: (成功時は nil、既存/失敗はエラー)
	EnsureBridge(vnet *api.VirtualNetwork) error

	// EnsureOverlayMesh はローカルノード上で、指定ピアへのトンネル群を作成・確認する。
	// 冪等で実行される。
	// vnet: 対象ネットワーク
	// peers: 疎通対象のリモートノード IP リスト
	EnsureOverlayMesh(vnet *api.VirtualNetwork, peers []string) error

	// PruneOverlayMesh は不要なトンネルを削除する。
	// 例: ノード離脱時に peer リストから外れたノードへのトンネルを削除。
	PruneOverlayMesh(vnet *api.VirtualNetwork, remainPeers []string) error

	// DeleteBridge はブリッジとその配下のトンネルをすべて削除する。
	// 削除前に必ず libvirt net destroy/undefine が完了していることを前提とする。
	DeleteBridge(vnet *api.VirtualNetwork) error

	// GetBridgeStatus はブリッジ存在状態と peer 数を返す。
	// 監視・drift 検知用。
	GetBridgeStatus(vnet *api.VirtualNetwork) (exists bool, peerCount int, err error)
}

// ACLRule は OVN の論理スイッチに適用するACL(アクセス制御リスト)1件分を表す。
type ACLRule struct {
	Direction string // "from-lport" または "to-lport"
	Priority  int    // 数値が大きいほど優先される(OVN仕様)
	Match     string // OVNのmatch式(例: "ip4 && ip4.dst==10.245.0.1 && tcp.dst==9090")
	Action    string // "allow" / "allow-related" / "drop" / "reject"
}

// ACLFabric は OVN ACLをサポートするFabricが実装するオプションの拡張インターフェース。
// OVSベースの非OVN Fabricは実装しなくてよい。
type ACLFabric interface {
	// EnsureACLs は対象ネットワークの論理スイッチ上のACLを rules の内容で完全に同期する。
	// 既存のACLはすべて削除してから rules を再作成する(冪等)。
	EnsureACLs(vnet *api.VirtualNetwork, rules []ACLRule) error
}

// PeerNode はメッシュ構成用のピア情報。
type PeerNode struct {
	NodeName   string // ノード名
	UnderlayIP string // underlay インターフェース IP
}
