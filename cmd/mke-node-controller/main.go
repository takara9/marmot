// Command mke-node-controller は、marmot上のKubernetesクラスタのkube-apiserverと連携し、
// pkg/cloudprovider.Instances(marmotd Server API連携)を用いてNodeのProviderID/Addressesを設定し、
// node.cloudprovider.kubernetes.io/uninitialized taintを解消する軽量reconcilerである。
// あわせて pkg/cloudprovider.LoadBalancer(marmotd loadbalancer/vip API連携)を用いて、
// Service type=LoadBalancerのVIP払い出し・解放も行う(フェーズ14項目5)。
// systemdユニットへの組み込み、kubeletの--cloud-provider=external切り替えは別変更セットで対応する。
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/cloudprovider"
)

func main() {
	kubeconfigPath := flag.String("kubeconfig", "/etc/marmot/mke-node-controller-kubeconfig", "Path to the kubeconfig used to access kube-apiserver")
	intervalSeconds := flag.Int("interval-seconds", 5, "Reconcile interval in seconds")
	marmotdURL := flag.String("marmotd-url", "", "Base URL of the marmotd REST API (e.g. http://192.168.1.10:8750)")
	marmotdApiKeyFile := flag.String("marmotd-apikey-file", "/etc/marmot/mke-node-controller-apikey", "Path to the marmotd API key token file")
	marmotdCAFile := flag.String("marmotd-ca-file", "", "Path to a PEM-encoded CA bundle used to verify marmotd over HTTPS")
	kubernetesEngineID := flag.String("kubernetes-engine-id", "", "ID of the KubernetesEngine this controller belongs to (required)")
	internalNetworkName := flag.String("internal-network", "", "marmotd network name used as Node InternalIP (required)")
	externalNetworkName := flag.String("external-network", "host-bridge", "marmotd network name used as Node ExternalIP (empty disables ExternalIP)")
	region := flag.String("region", "", "Region reported in Node InstanceMetadata (empty disables Region)")
	zone := flag.String("zone", "", "Zone reported in Node InstanceMetadata (empty disables Zone)")
	flag.Parse()

	if *intervalSeconds <= 0 {
		log.Fatalf("interval-seconds must be positive")
	}
	if strings.TrimSpace(*kubernetesEngineID) == "" {
		log.Fatalf("kubernetes-engine-id is required")
	}
	if strings.TrimSpace(*internalNetworkName) == "" {
		log.Fatalf("internal-network is required")
	}
	if strings.TrimSpace(*marmotdURL) == "" {
		log.Fatalf("marmotd-url is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	kc, err := loadKubeClient(*kubeconfigPath)
	if err != nil {
		log.Fatalf("failed to load kubeconfig %q: %v", *kubeconfigPath, err)
	}

	endpoint, err := loadMarmotdEndpoint(*marmotdURL, *marmotdApiKeyFile, *marmotdCAFile)
	if err != nil {
		log.Fatalf("failed to initialize marmotd endpoint: %v", err)
	}
	instances := cloudprovider.NewMarmotdInstances(endpoint, *kubernetesEngineID, *internalNetworkName, *externalNetworkName, *region, *zone)
	loadBalancer := cloudprovider.NewMarmotdLoadBalancer(endpoint)
	knownServiceVIPs := make(map[string]struct{})

	ticker := time.NewTicker(time.Duration(*intervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		reconcile(ctx, kc, instances)
		reconcileLoadBalancer(ctx, kc, loadBalancer, *kubernetesEngineID, knownServiceVIPs)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// loadMarmotdEndpoint は、apiKeyFileからmarmotd APIKeyトークンを読み込み、必要に応じて
// 追加のCA証明書でHTTPS接続を検証しながらMarmotEndpointを組み立てる。
func loadMarmotdEndpoint(rawURL, apiKeyFile, caFile string) (*client.MarmotEndpoint, error) {
	raw, err := os.ReadFile(apiKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read marmotd API key file %q: %w", apiKeyFile, err)
	}
	apiKey := strings.TrimSpace(string(raw))
	if apiKey == "" {
		return nil, fmt.Errorf("marmotd API key file %q is empty", apiKeyFile)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid marmotd-url %q: %w", rawURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("marmotd-url %q must include scheme and host", rawURL)
	}

	endpoint, err := client.NewMarmotdEp(u.Scheme, u.Host, "/api/v1", 10, false)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(caFile) != "" {
		pemData, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read marmotd CA file %q: %w", caFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("failed to parse marmotd CA file %q", caFile)
		}
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.DisableCompression = true
		tr.TLSClientConfig = &tls.Config{RootCAs: pool}
		endpoint.Client.Transport = tr
	}

	endpoint.AccessToken = apiKey
	return endpoint, nil
}
