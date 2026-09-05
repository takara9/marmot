// Command mke-lb-controller は、mke専用ロードバランサー仮想サーバー上で動作し、
// kube-apiserverを監視してLoadBalancer種別のServiceとNodeの情報を収集し、
// VIPが払い出し済みのServiceについてHAProxy設定を生成・検証・reloadする。
// VIPの払い出し・内部DNS登録は別変更セットで実装する。
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	kubeconfigPath := flag.String("kubeconfig", "/etc/marmot/mke-lb-kubeconfig", "Path to the kubeconfig used to access kube-apiserver")
	intervalSeconds := flag.Int("interval-seconds", 5, "Reconcile interval in seconds")
	haproxyConfigPath := flag.String("haproxy-config", "/etc/haproxy/haproxy.cfg", "Path to the HAProxy config file to manage")
	marmotdURL := flag.String("marmotd-url", "", "Base URL of the marmotd REST API (e.g. http://192.168.1.10:8750)")
	marmotdApiKeyFile := flag.String("marmotd-apikey-file", "/etc/marmot/mke-lb-apikey", "Path to the marmotd API key token file")
	marmotdCAFile := flag.String("marmotd-ca-file", "", "Path to a PEM-encoded CA bundle used to verify marmotd over HTTPS")
	kubernetesEngineID := flag.String("kubernetes-engine-id", "", "ID of the KubernetesEngine this load balancer belongs to (required for VIP allocation)")
	vipInterface := flag.String("vip-interface", "", "Network interface on this load balancer VM to assign VIP addresses to (required for HAProxy to bind to the VIP)")
	cloudControllerManagerEnabled := flag.Bool("cloud-controller-manager-enabled", false, "Set when the cluster's cloud-controller-manager (mke-node-controller) owns Node status.addresses; disables this controller's own SetNodeAddresses calls to avoid conflicting writes")
	flag.Parse()

	if *intervalSeconds <= 0 {
		log.Fatalf("interval-seconds must be positive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := loadKubeClient(*kubeconfigPath)
	if err != nil {
		log.Fatalf("failed to load kubeconfig %q: %v", *kubeconfigPath, err)
	}

	var mClient *marmotdClient
	if strings.TrimSpace(*marmotdURL) == "" {
		log.Printf("marmotd-url is not set, host-bridge address collection and VIP allocation are disabled")
	} else if mClient, err = loadMarmotdClient(*marmotdURL, *marmotdApiKeyFile, *kubernetesEngineID, *marmotdCAFile); err != nil {
		log.Fatalf("failed to initialize marmotd client")
	}

	ticker := time.NewTicker(time.Duration(*intervalSeconds) * time.Second)
	defer ticker.Stop()

	if strings.TrimSpace(*vipInterface) == "" {
		log.Printf("vip-interface is not set, VIP will not be assigned to a network interface on this host")
	}

	lastAppliedHash := ""
	knownServiceVIPs := map[string]string{}
	for {
		lastAppliedHash, knownServiceVIPs = reconcile(ctx, client, mClient, *haproxyConfigPath, *vipInterface, lastAppliedHash, knownServiceVIPs, *cloudControllerManagerEnabled)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func reconcile(ctx context.Context, client *kubeClient, mClient *marmotdClient, haproxyConfigPath, vipInterface, lastAppliedHash string, knownServiceVIPs map[string]string, cloudControllerManagerEnabled bool) (string, map[string]string) {
	nodes, err := client.ListNodes(ctx)
	if err != nil {
		log.Printf("failed to list nodes: %v", err)
	} else {
		for _, node := range nodes {
			log.Printf("node observed: name=%s internalIP=%s", node.Name, node.InternalIP)
		}
	}

	// HAProxy backendはhost-bridge接続アドレスを使う(フェーズ11)。marmotd未設定、または
	// host-bridgeアドレスが見つからないノードは、誤った経路への転送を避けるためbackendから除外する。
	haproxyNodes := nodes
	if mClient != nil {
		hostBridgeAddresses, hbErr := mClient.hostBridgeAddressesByServerName(ctx)
		if hbErr != nil {
			log.Printf("failed to collect host-bridge addresses from marmotd: %v", hbErr)
		}
		haproxyNodes = make([]nodeInfo, 0, len(nodes))
		for _, node := range nodes {
			addr, ok := hostBridgeAddresses[node.Name]
			if !ok {
				log.Printf("node host-bridge address not found in marmotd, excluding from HAProxy backend: name=%s", node.Name)
				continue
			}
			log.Printf("node host-bridge address observed: name=%s hostBridgeAddress=%s", node.Name, addr)
			haproxyNodes = append(haproxyNodes, nodeInfo{Name: node.Name, InternalIP: addr})

			// cloud-controller-manager(mke-node-controller)がNode.status.addressesを設定する
			// クラスタでは、この呼び出しと競合するため行わない(フェーズ14項目4の排他切り替え)。
			if cloudControllerManagerEnabled {
				continue
			}
			if node.ExternalIP != addr {
				if err := client.SetNodeAddresses(ctx, node.Name, node.InternalIP, addr); err != nil {
					log.Printf("failed to set ExternalIP for node %s: %v", node.Name, err)
				} else {
					log.Printf("node ExternalIP set: name=%s externalIP=%s", node.Name, addr)
				}
			}
		}
	}

	services, err := client.ListLoadBalancerServices(ctx)
	if err != nil {
		log.Printf("failed to list LoadBalancer services: %v", err)
		return lastAppliedHash, knownServiceVIPs
	}

	currentServiceVIPs := make(map[string]string, len(services))
	for i := range services {
		svc := &services[i]

		if svc.VIP == "" && mClient != nil && !cloudControllerManagerEnabled {
			vip, err := mClient.requestVip(ctx, svc.Namespace, svc.Name)
			if err != nil {
				log.Printf("failed to request VIP for service %s/%s: %v", svc.Namespace, svc.Name, err)
			} else if err := client.SetServiceLoadBalancerIngressIP(ctx, svc.Namespace, svc.Name, vip); err != nil {
				log.Printf("failed to set EXTERNAL-IP for service %s/%s: %v", svc.Namespace, svc.Name, err)
			} else {
				svc.VIP = vip
				log.Printf("VIP allocated and set: namespace=%s name=%s vip=%s", svc.Namespace, svc.Name, vip)
			}
		}

		if svc.VIP == "" {
			log.Printf("LoadBalancer service %s/%s has no VIP assigned yet, skipping", svc.Namespace, svc.Name)
			continue
		}
		currentServiceVIPs[svc.Namespace+"/"+svc.Name] = svc.VIP
		log.Printf("LoadBalancer service observed: namespace=%s name=%s vip=%s ports=%d", svc.Namespace, svc.Name, svc.VIP, len(svc.Ports))

		if vipInterface != "" {
			if err := ensureVipAddress(vipInterface, svc.VIP); err != nil {
				log.Printf("failed to assign VIP %s to interface %s: %v", svc.VIP, vipInterface, err)
			}
		}
	}

	// cloud-controller-manager(mke-node-controller)がVIPの払い出し/解放を担うクラスタでは、
	// この呼び出しと竞合するため行わない(フェーズ14項目5の排他切り替え)。
	if mClient != nil && !cloudControllerManagerEnabled {
		for key, vip := range knownServiceVIPs {
			if _, ok := currentServiceVIPs[key]; ok {
				continue
			}
			namespace, name, ok := strings.Cut(key, "/")
			if !ok {
				continue
			}
			if err := mClient.releaseVip(ctx, namespace, name); err != nil {
				log.Printf("failed to release VIP for removed service %s/%s: %v", namespace, name, err)
				continue
			}
			log.Printf("VIP released for removed service: namespace=%s name=%s", namespace, name)
			if vipInterface != "" && vip != "" {
				if err := removeVipAddress(vipInterface, vip); err != nil {
					log.Printf("failed to remove VIP %s from interface %s: %v", vip, vipInterface, err)
				}
			}
		}
	}

	config := renderHAProxyConfig(haproxyNodes, services)
	newHash, changed, err := applyHAProxyConfig(haproxyConfigPath, config, lastAppliedHash)
	if err != nil {
		log.Printf("failed to apply HAProxy config: %v", err)
		return lastAppliedHash, currentServiceVIPs
	}
	if changed {
		log.Printf("applied HAProxy config: hash=%s", newHash)
	}
	return newHash, currentServiceVIPs
}
