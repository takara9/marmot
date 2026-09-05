package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

// kubernetesEngineLoadBalancerVipDnsSuffix は、フェーズ11仕様の内部DNS命名規則
// (サービス名.ネームスペース名.MKEクラスタ名.HVホスト名.labo.local)のうち、
// クラスタ名・ホスト名以降の共通接尾辞を返す。
func kubernetesEngineLoadBalancerVipDnsSuffix(ke api.KubernetesEngine) (string, bool) {
	if ke.Metadata.NodeName == nil || strings.TrimSpace(*ke.Metadata.NodeName) == "" {
		return "", false
	}
	clusterName := strings.TrimSpace(ke.Metadata.Name)
	if clusterName == "" {
		return "", false
	}
	return fmt.Sprintf("%s.%s.labo.local", clusterName, strings.TrimSpace(*ke.Metadata.NodeName)), true
}

// releaseKubernetesEngineLoadBalancerVips は、このクラスタがフェーズ11の仕組みで払い出した
// 全VIP(host-bridge IPAMプール)と内部DNSエントリーを解放する。
// クラスタ全体の削除時は、mke専用ロードバランサー用仮想サーバー(mke-lb-controller)が
// Serviceの消失を検知して個別に解放する機会を失うため、ここで一括解放する。
// 対象が無い場合は何もせず成功として扱う(冪等)。
func releaseKubernetesEngineLoadBalancerVips(database *db.Database, ke api.KubernetesEngine) error {
	suffix, ok := kubernetesEngineLoadBalancerVipDnsSuffix(ke)
	if !ok {
		return nil
	}
	entries, err := database.ListDnsEntriesByFQDNSuffix(suffix)
	if err != nil {
		return fmt.Errorf("list load balancer VIP DNS entries: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}

	vnet, err := database.GetVirtualNetworkByName("host-bridge")
	if err != nil {
		return fmt.Errorf("get host-bridge network: %w", err)
	}

	var errs []error
	for fqdn, vip := range entries {
		if err := database.DeleteDnsEntryFQDN(fqdn); err != nil {
			errs = append(errs, fmt.Errorf("delete DNS entry %s: %w", fqdn, err))
			continue
		}
		if vnet.Spec.IpNetworkId == nil || strings.TrimSpace(*vnet.Spec.IpNetworkId) == "" {
			errs = append(errs, fmt.Errorf("host-bridge network has no ipNetworkId; cannot release VIP %s (%s)", vip, fqdn))
			continue
		}
		if err := database.ReleaseIP(api.VirtualNetworkID(vnet), strings.TrimSpace(*vnet.Spec.IpNetworkId), vip); err != nil && !errors.Is(err, db.ErrNotFound) {
			errs = append(errs, fmt.Errorf("release VIP %s (%s): %w", vip, fqdn, err))
		}
	}
	return errors.Join(errs...)
}
