package controller

import (
	"fmt"
	"strings"
)

// provisionKubernetesEngineLoadBalancerSSH は mke専用ロードバランサー仮想サーバーへSSH接続し、
// HA-Proxyのインストール、ロードバランサーコントローラー(mke-lb-controller)の配布・起動、
// kube-apiserverへアクセスするためのkubeconfigの配布を行う。ノードと同様、コントロールプレーン用
// netns経由で「ノード間通信用ネットワーク」上のアドレスへ到達する。
func provisionKubernetesEngineLoadBalancerSSH(address, privateKeyPath, namespace, resourceID string, kubeconfig, controllerBinary []byte, apiKeyToken, marmotdURL, marmotdCAFile string, marmotdCACert []byte, kubernetesEngineID string, cloudControllerManagerEnabled bool) error {
	client, err := dialKubernetesEngineNodeSSH(address, privateKeyPath, namespace)
	if err != nil {
		return fmt.Errorf("failed to connect to load balancer server: %w", err)
	}
	defer func() { _ = client.Close() }()

	runner := &kubernetesEngineNodeSSHRunner{client: client, resourceID: resourceID}
	if err := runner.step("install HA-Proxy", func() error {
		return runner.run("DEBIAN_FRONTEND=noninteractive apt-get update && "+
			"DEBIAN_FRONTEND=noninteractive apt-get install -y haproxy && "+
			"systemctl enable --now haproxy", nil)
	}); err != nil {
		return err
	}

	// VM再起動直後、VIPがNICへ割り当てられるより先にHAProxyが起動してbindに失敗し、
	// systemdの再起動制限でfailed固着することを防ぐ(起動順序に依存しない恒久対策)。
	if err := runner.step("allow HAProxy to bind the VIP before it is assigned to the NIC", func() error {
		return runner.writeFile(kubernetesEngineLoadBalancerSysctlPath, "0644",
			[]byte("net.ipv4.ip_nonlocal_bind = 1\n"))
	}); err != nil {
		return err
	}
	if err := runner.step("apply non-local bind sysctl", func() error {
		return runner.run(fmt.Sprintf("sysctl -p %s", kubernetesEngineLoadBalancerSysctlPath), nil)
	}); err != nil {
		return err
	}

	if err := runner.step("write load balancer kubeconfig", func() error {
		return runner.writeFile(kubernetesEngineLoadBalancerKubeconfigPath, "0600", kubeconfig)
	}); err != nil {
		return err
	}

	if err := runner.step("install mke-lb-controller binary", func() error {
		return runner.writeFile(kubernetesEngineLoadBalancerControllerBinaryDestPath, "0755", controllerBinary)
	}); err != nil {
		return err
	}

	if err := runner.step("write mke-lb-controller marmotd API key", func() error {
		return runner.writeFile(kubernetesEngineLoadBalancerApiKeyPath, "0600", []byte(apiKeyToken))
	}); err != nil {
		return err
	}
	if len(marmotdCACert) > 0 {
		if err := runner.step("write mke-lb-controller marmotd CA bundle", func() error {
			return runner.writeFile(marmotdCAFile, "0644", marmotdCACert)
		}); err != nil {
			return err
		}
	}

	if err := runner.step("install mke-lb-controller systemd unit", func() error {
		return runner.writeFile(kubernetesEngineLoadBalancerControllerSystemdUnitPath, "0644",
			[]byte(kubernetesEngineLoadBalancerControllerUnit(marmotdURL, marmotdCAFile, kubernetesEngineID, cloudControllerManagerEnabled)))
	}); err != nil {
		return err
	}

	return runner.step("enable mke-lb-controller service", func() error {
		return runner.run("systemctl daemon-reload && systemctl enable --now mke-lb-controller", nil)
	})
}

func kubernetesEngineLoadBalancerControllerUnit(marmotdURL, marmotdCAFile, kubernetesEngineID string, cloudControllerManagerEnabled bool) string {
	caArgs := ""
	if strings.TrimSpace(marmotdCAFile) != "" {
		caArgs = fmt.Sprintf(" --marmotd-ca-file=%s", marmotdCAFile)
	}
	cloudControllerManagerArg := ""
	if cloudControllerManagerEnabled {
		// CCM(mke-node-controller)がNode.status.addressesを設定するクラスタでは、
		// mke-lb-controller側のSetNodeAddresses呼び出しと競合するため無効化する(フェーズ14項目4)。
		cloudControllerManagerArg = " --cloud-controller-manager-enabled=true"
	}
	return fmt.Sprintf(`[Unit]
Description=MKE Load Balancer Controller
Wants=network-online.target haproxy.service
After=network-online.target haproxy.service
 
[Service]
ExecStart=%s --kubeconfig=%s --haproxy-config=%s --marmotd-url=%s --marmotd-apikey-file=%s%s --kubernetes-engine-id=%s --vip-interface=enp1s0%s
Restart=always
RestartSec=5
 
[Install]
WantedBy=multi-user.target
`, kubernetesEngineLoadBalancerControllerBinaryDestPath, kubernetesEngineLoadBalancerKubeconfigPath, kubernetesEngineLoadBalancerHAProxyConfigPath, marmotdURL, kubernetesEngineLoadBalancerApiKeyPath, caArgs, kubernetesEngineID, cloudControllerManagerArg)
}
