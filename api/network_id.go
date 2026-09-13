package api

// NetworkLabelACLEnforced はネットワークのMetadata.Labelsに設定するラベルキー。
// "true" が設定されたネットワークは、OVN ACLを実トラフィックに適用するため、
// ゲストNICをOVN論理ポートとして正式に束縛し、生geneveトンネルメッシュへの
// フォールバックを行わない(issue #696)。
const NetworkLabelACLEnforced = "marmot_acl_enforced"

// OVNIntegrationBridgeName は ovn-controller が実際にフロー制御する統合ブリッジ名。
// ACL適用ネットワークのゲストNICは、専用ブリッジではなくこのブリッジへ直接接続する
// 必要がある(issue #696)。Marmotが作成・削除の対象とする通常のネットワーク専用
// ブリッジとは異なり、既存の共有インフラとして扱い、作成・削除処理の対象としない。
const OVNIntegrationBridgeName = "br-int"

// VirtualNetworkID returns the virtual network identifier stored in metadata.id.
func VirtualNetworkID(v VirtualNetwork) string {
	return v.Metadata.Id
}

// SetVirtualNetworkID stores the virtual network identifier into metadata.id.
func SetVirtualNetworkID(v *VirtualNetwork, id string) {
	if v == nil {
		return
	}
	v.Metadata.Id = id
}
