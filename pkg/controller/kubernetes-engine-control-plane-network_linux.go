package controller

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

const controlPlaneInterfaceName = "eth0"

type KubernetesEngineControlPlaneNetworkConfig struct {
	ClusterName  string
	Namespace    string
	HostVethName string
	PeerVethName string
	BridgeName   string
	Interface    string
	CIDR         string
	// HostCIDR は hostVeth (ホスト側/ルートnetns側) に割り当てるIPアドレス(CIDR形式)。
	// コントロールプレーンnetnsのIPアドレスと同一サブネット内の別アドレスを指定することで、
	// ルートnetnsからコントロールプレーンnetnsへの経路が確立され、
	// kube-apiserverへのDNAT転送が可能になる。空の場合は付与しない(後方互換)。
	HostCIDR string
}

var (
	controlPlaneNamespaceExists = func(name string) bool {
		handle, err := netns.GetFromName(name)
		if err != nil {
			return false
		}
		_ = handle.Close()
		return true
	}
	controlPlaneCreateNamespace = createControlPlaneNamespace
	controlPlaneDeleteNamespace = netns.DeleteNamed
	controlPlaneRunIP           = func(args ...string) ([]byte, error) {
		return exec.Command("ip", args...).CombinedOutput()
	}
	controlPlaneCreateVeth = createControlPlaneVeth
	controlPlaneAddOVSPort = func(bridge, port string) error {
		output, err := exec.Command("ovs-vsctl", "--may-exist", "add-port", bridge, port).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ovs-vsctl add-port failed: %w (output=%s)", err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	controlPlaneDeleteOVSPort = func(bridge, port string) error {
		output, err := exec.Command("ovs-vsctl", "--if-exists", "del-port", bridge, port).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ovs-vsctl del-port failed: %w (output=%s)", err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	controlPlaneSetHostVethUp = func(name string) error {
		link, err := netlink.LinkByName(name)
		if err != nil {
			return err
		}
		return netlink.LinkSetUp(link)
	}
	controlPlaneSetHostVethAddress = func(name, cidr string) error {
		link, err := netlink.LinkByName(name)
		if err != nil {
			return err
		}
		address, err := netlink.ParseAddr(cidr)
		if err != nil {
			return err
		}
		return netlink.AddrAdd(link, address)
	}
	controlPlaneConfigurePeer = configureControlPlanePeer
)

func KubernetesEngineControlPlaneNetworkNames(clusterName string) (namespace, hostVeth, peerVeth string, err error) {
	name, err := validateKubernetesEngineEtcdClusterName(clusterName)
	if err != nil {
		return "", "", "", err
	}
	sum := sha256.Sum256([]byte(name))
	suffix := fmt.Sprintf("%x", sum[:5])
	return "mke-" + name, "mkoh-" + suffix, "mken-" + suffix, nil
}

func NewKubernetesEngineControlPlaneNetworkConfig(clusterName, bridgeName, cidr string) (KubernetesEngineControlPlaneNetworkConfig, error) {
	namespace, hostVeth, peerVeth, err := KubernetesEngineControlPlaneNetworkNames(clusterName)
	if err != nil {
		return KubernetesEngineControlPlaneNetworkConfig{}, err
	}
	if strings.TrimSpace(bridgeName) == "" {
		return KubernetesEngineControlPlaneNetworkConfig{}, fmt.Errorf("bridge name is empty")
	}
	if _, _, err := net.ParseCIDR(strings.TrimSpace(cidr)); err != nil {
		return KubernetesEngineControlPlaneNetworkConfig{}, fmt.Errorf("invalid control plane CIDR %q: %w", cidr, err)
	}
	return KubernetesEngineControlPlaneNetworkConfig{
		ClusterName:  strings.TrimSpace(clusterName),
		Namespace:    namespace,
		HostVethName: hostVeth,
		PeerVethName: peerVeth,
		BridgeName:   strings.TrimSpace(bridgeName),
		Interface:    controlPlaneInterfaceName,
		CIDR:         strings.TrimSpace(cidr),
	}, nil
}

func SetupKubernetesEngineControlPlaneNetwork(cfg KubernetesEngineControlPlaneNetworkConfig) (err error) {
	if _, err := NewKubernetesEngineControlPlaneNetworkConfig(cfg.ClusterName, cfg.BridgeName, cfg.CIDR); err != nil {
		return err
	}
	if controlPlaneNamespaceExists(cfg.Namespace) {
		// If the namespace exists, treat it as ready only when the expected host veth exists.
		if _, err := netlink.LinkByName(cfg.HostVethName); err == nil {
			return nil
		}
		_ = controlPlaneDeleteOVSPort(cfg.BridgeName, cfg.HostVethName)
		if err := controlPlaneDeleteNamespace(cfg.Namespace); err != nil {
			return fmt.Errorf("failed to delete stale network namespace %s: %w", cfg.Namespace, err)
		}
	}
	if err := controlPlaneCreateNamespace(cfg.Namespace); err != nil {
		return fmt.Errorf("failed to create network namespace %s: %w", cfg.Namespace, err)
	}
	defer func() {
		if err != nil {
			_ = controlPlaneDeleteOVSPort(cfg.BridgeName, cfg.HostVethName)
			_ = controlPlaneDeleteNamespace(cfg.Namespace)
		}
	}()
	if err = controlPlaneCreateVeth(cfg.HostVethName, cfg.PeerVethName); err != nil {
		return fmt.Errorf("failed to create veth pair: %w", err)
	}
	if err = controlPlaneAddOVSPort(cfg.BridgeName, cfg.HostVethName); err != nil {
		return err
	}
	if err = controlPlaneSetHostVethUp(cfg.HostVethName); err != nil {
		return fmt.Errorf("failed to bring host veth up: %w", err)
	}
	if strings.TrimSpace(cfg.HostCIDR) != "" {
		if err = controlPlaneSetHostVethAddress(cfg.HostVethName, cfg.HostCIDR); err != nil {
			return fmt.Errorf("failed to assign host veth address: %w", err)
		}
	}
	if err = controlPlaneConfigurePeer(cfg.Namespace, cfg.PeerVethName, cfg.Interface, cfg.CIDR); err != nil {
		return fmt.Errorf("failed to configure network namespace: %w", err)
	}
	return nil
}

func TeardownKubernetesEngineControlPlaneNetwork(cfg KubernetesEngineControlPlaneNetworkConfig) error {
	if err := controlPlaneDeleteOVSPort(cfg.BridgeName, cfg.HostVethName); err != nil {
		return err
	}
	if err := controlPlaneDeleteNamespace(cfg.Namespace); err != nil {
		return fmt.Errorf("failed to delete network namespace %s: %w", cfg.Namespace, err)
	}
	return nil
}

func createControlPlaneNamespace(name string) error {
	output, err := controlPlaneRunIP("netns", "add", name)
	if err != nil {
		return fmt.Errorf("ip netns add failed: %w (output=%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func createControlPlaneVeth(hostName, peerName string) error {
	return netlink.LinkAdd(&netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: hostName},
		PeerName:  peerName,
	})
}

func configureControlPlanePeer(namespace, peerName, interfaceName, cidr string) error {
	peer, err := netlink.LinkByName(peerName)
	if err != nil {
		return err
	}
	target, err := netns.GetFromName(namespace)
	if err != nil {
		return err
	}
	defer func() { _ = target.Close() }()
	if err := netlink.LinkSetNsFd(peer, int(target)); err != nil {
		return err
	}

	handle, err := netlink.NewHandleAt(target)
	if err != nil {
		return err
	}
	defer handle.Close()

	link, err := handle.LinkByName(peerName)
	if err != nil {
		return err
	}
	if err := handle.LinkSetName(link, interfaceName); err != nil {
		return err
	}
	link, err = handle.LinkByName(interfaceName)
	if err != nil {
		return err
	}
	address, err := netlink.ParseAddr(cidr)
	if err != nil {
		return err
	}
	if err := handle.AddrAdd(link, address); err != nil {
		return err
	}
	if err := handle.LinkSetUp(link); err != nil {
		return err
	}
	loopback, err := handle.LinkByName("lo")
	if err != nil {
		return err
	}
	return handle.LinkSetUp(loopback)
}

var (
	dialNamespaceGetCurrent = netns.Get
	dialNamespaceGetByName  = netns.GetFromName
	dialNamespaceSet        = netns.Set
	dialNamespaceNetDial    = func(network, address string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout(network, address, timeout)
	}
)

type kubernetesEngineNamespaceDialResult struct {
	conn net.Conn
	err  error
}

// DialInKubernetesEngineNetworkNamespace は指定されたネットワーク名前空間の中でTCP接続を確立する。
// namespaceが空の場合は現在の名前空間からダイヤルする。コントロールプレーン用に作成済みの
// netns(veth経由でOVSブリッジに接続済み)を再利用し、ノード間通信用ネットワーク上のIPへの
// 到達性をホスト側プロセスからも得るために使用する。
func DialInKubernetesEngineNetworkNamespace(namespace, network, address string, timeout time.Duration) (net.Conn, error) {
	if strings.TrimSpace(namespace) == "" {
		return dialNamespaceNetDial(network, address, timeout)
	}

	resultCh := make(chan kubernetesEngineNamespaceDialResult, 1)
	go func() {
		// 名前空間の切り替えは呼び出しスレッド全体に影響するため、
		// 他のgoroutineに影響しないよう専用OSスレッドに固定する。
		runtime.LockOSThread()

		origNS, err := dialNamespaceGetCurrent()
		if err != nil {
			runtime.UnlockOSThread()
			resultCh <- kubernetesEngineNamespaceDialResult{err: fmt.Errorf("failed to get current network namespace: %w", err)}
			return
		}
		defer func() { _ = origNS.Close() }()

		targetNS, err := dialNamespaceGetByName(namespace)
		if err != nil {
			runtime.UnlockOSThread()
			resultCh <- kubernetesEngineNamespaceDialResult{err: fmt.Errorf("failed to open network namespace %s: %w", namespace, err)}
			return
		}
		defer func() { _ = targetNS.Close() }()

		if err := dialNamespaceSet(targetNS); err != nil {
			runtime.UnlockOSThread()
			resultCh <- kubernetesEngineNamespaceDialResult{err: fmt.Errorf("failed to enter network namespace %s: %w", namespace, err)}
			return
		}

		conn, dialErr := dialNamespaceNetDial(network, address, timeout)

		if restoreErr := dialNamespaceSet(origNS); restoreErr != nil {
			// 元の名前空間へ復帰できなかったOSスレッドは再利用させず終了させる(UnlockOSThreadを呼ばない)。
			if conn != nil {
				_ = conn.Close()
			}
			resultCh <- kubernetesEngineNamespaceDialResult{err: fmt.Errorf("failed to restore original network namespace: %w", restoreErr)}
			return
		}

		runtime.UnlockOSThread()
		resultCh <- kubernetesEngineNamespaceDialResult{conn: conn, err: dialErr}
	}()

	result := <-resultCh
	return result.conn, result.err
}
