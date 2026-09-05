package main

import (
	"context"
	"log"
	"strings"

	"github.com/takara9/marmot/pkg/cloudprovider"
)

// kubernetesEngineNodeUninitializedTaintKey は、kubeletが --cloud-provider=external 設定時に
// 起動直後のNodeへ自動付与するtaint。cloud-controller-manager相当の存在がProviderID/Addressesを
// セットしてNode初期化を完了させると、このtaintを解消する規約になっている。
const kubernetesEngineNodeUninitializedTaintKey = "node.cloudprovider.kubernetes.io/uninitialized"

// reconcile は、kube-apiserverから取得した全Nodeについて、instancesから得たProviderID/Addressesを
// 反映し、uninitialized taintを解消する。各ステップはベストエフォートで、失敗しても他のNode/処理を
// 継続する(次回のreconcileで再試行される)。
func reconcile(ctx context.Context, kc *kubeClient, instances cloudprovider.Instances) {
	nodes, err := kc.ListNodes(ctx)
	if err != nil {
		log.Printf("failed to list nodes: %v", err)
		return
	}

	for _, node := range nodes {
		meta, err := instances.InstanceMetadata(ctx, node.Name)
		if err != nil {
			log.Printf("failed to fetch instance metadata for node %s: %v", node.Name, err)
			continue
		}

		var desiredInternalIP, desiredExternalIP string
		for _, addr := range meta.NodeAddresses {
			switch addr.Type {
			case cloudprovider.NodeInternalIP:
				desiredInternalIP = addr.Address
			case cloudprovider.NodeExternalIP:
				desiredExternalIP = addr.Address
			}
		}
		if desiredInternalIP == "" {
			log.Printf("skipping node address update (InternalIP missing in instance metadata): name=%s", node.Name)
		} else if node.InternalIP != desiredInternalIP || node.ExternalIP != desiredExternalIP {
			if err := kc.SetNodeAddresses(ctx, node.Name, desiredInternalIP, desiredExternalIP); err != nil {
				log.Printf("failed to set addresses for node %s: %v", node.Name, err)
			} else {
				log.Printf("node addresses set: name=%s internalIP=%s externalIP=%s", node.Name, desiredInternalIP, desiredExternalIP)
			}
		}

		remainingTaints, hadUninitializedTaint := removeTaint(node.Taints, kubernetesEngineNodeUninitializedTaintKey)
		providerID := node.ProviderID
		needsProviderIDUpdate := meta.ProviderID != "" && node.ProviderID != meta.ProviderID
		if needsProviderIDUpdate {
			providerID = meta.ProviderID
		}
		if needsProviderIDUpdate || hadUninitializedTaint {
			if err := kc.PatchNodeSpec(ctx, node.Name, providerID, remainingTaints); err != nil {
				log.Printf("failed to patch spec for node %s: %v", node.Name, err)
			} else {
				log.Printf("node spec patched: name=%s providerID=%s taintRemoved=%t", node.Name, providerID, hadUninitializedTaint)
			}
		}
	}
}

// removeTaint は taints から key に一致する要素を取り除いた新しいスライスと、
// 実際に除去が発生したかどうかを返す。
func removeTaint(taints []nodeTaint, key string) ([]nodeTaint, bool) {
	out := make([]nodeTaint, 0, len(taints))
	removed := false
	for _, t := range taints {
		if t.Key == key {
			removed = true
			continue
		}
		out = append(out, t)
	}
	return out, removed
}

// serviceKey は namespace/name の組み合わせをmapキーとして扱うための文字列表現。
func serviceKey(namespace, name string) string {
	return namespace + "/" + name
}

// reconcileLoadBalancer は、kube-apiserverから取得したType=LoadBalancerの全Serviceについて、
// VIP未払い出し(EXTERNAL-IP未設定)ならlbで払い出してSetServiceLoadBalancerIngressIPで反映し、
// 前回observe時に存在したが今回は消えたServiceについてはVIPを解放する
// (フェーズ11の「ロードバランサーコントローラー」のVIP管理ロジックをCCM相当に移植したもの)。
// knownServiceVIPsは呼び出し側がreconcileループを跨いで保持するマップで、この関数が更新する。
func reconcileLoadBalancer(ctx context.Context, kc *kubeClient, lb cloudprovider.LoadBalancer, clusterID string, knownServiceVIPs map[string]struct{}) {
	services, err := kc.ListLoadBalancerServices(ctx)
	if err != nil {
		log.Printf("failed to list load balancer services: %v", err)
		return
	}

	seen := make(map[string]struct{}, len(services))
	for _, svc := range services {
		key := serviceKey(svc.Namespace, svc.Name)
		seen[key] = struct{}{}
		if svc.VIP != "" {
			knownServiceVIPs[key] = struct{}{}
			continue
		}
		vip, err := lb.EnsureLoadBalancer(ctx, clusterID, cloudprovider.LoadBalancerService{Namespace: svc.Namespace, Name: svc.Name})
		if err != nil {
			log.Printf("failed to ensure load balancer for %s: %v", key, err)
			continue
		}
		if err := kc.SetServiceLoadBalancerIngressIP(ctx, svc.Namespace, svc.Name, vip); err != nil {
			log.Printf("failed to set load balancer ingress IP for %s: %v", key, err)
			continue
		}
		knownServiceVIPs[key] = struct{}{}
		log.Printf("load balancer VIP allocated: service=%s vip=%s", key, vip)
	}

	for key := range knownServiceVIPs {
		if _, ok := seen[key]; ok {
			continue
		}
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			delete(knownServiceVIPs, key)
			continue
		}
		if err := lb.EnsureLoadBalancerDeleted(ctx, clusterID, cloudprovider.LoadBalancerService{Namespace: parts[0], Name: parts[1]}); err != nil {
			log.Printf("failed to release load balancer VIP for %s: %v", key, err)
			continue
		}
		delete(knownServiceVIPs, key)
		log.Printf("load balancer VIP released: service=%s", key)
	}
}
