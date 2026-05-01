package networkfabric

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/takara9/marmot/api"
)

// OVSFabric はシェルコマンドベースの OVS/VXLAN 実装。
// 本番環境では将来 libopenswitch バインディングへの切り替えを前提に、
// ここではコマンド呼び出しでプロトタイピングする。
type OVSFabric struct {
	// 必要に応じて設定を保持（例: underlay_interface）
}

const splitHorizonCookie = "0x6d61726d6f740001"

// NewOVSFabric は OVSFabric インスタンスを生成する。
func NewOVSFabric() *OVSFabric {
	return &OVSFabric{}
}

// EnsureBridge は OVS ブリッジを作成または確認する。
func (o *OVSFabric) EnsureBridge(vnet *api.VirtualNetwork) error {
	if vnet == nil || vnet.Spec == nil || vnet.Spec.BridgeName == nil {
		return fmt.Errorf("invalid vnet or missing bridge name")
	}

	bridgeName := *vnet.Spec.BridgeName

	// ブリッジ存在確認
	cmd := exec.Command("ovs-vsctl", "br-exists", bridgeName)
	if err := cmd.Run(); err == nil {
		// ブリッジ既存
		slog.Debug("OVS bridge already exists", "bridge", bridgeName)
		return nil
	}

	// ブリッジ作成
	cmd = exec.Command("ovs-vsctl", "add-br", bridgeName)
	if output, err := cmd.CombinedOutput(); err != nil {
		slog.Error("failed to create OVS bridge", "bridge", bridgeName, "err", err, "output", string(output))
		return fmt.Errorf("failed to create bridge %s: %w", bridgeName, err)
	}

	slog.Info("OVS bridge created", "bridge", bridgeName)
	return nil
}

// EnsureVxlanMesh はピアノードへの VXLAN トンネルを作成・確認する。
func (o *OVSFabric) EnsureVxlanMesh(vnet *api.VirtualNetwork, peers []string) error {
	if vnet == nil || vnet.Spec == nil || vnet.Spec.BridgeName == nil {
		return fmt.Errorf("invalid vnet or missing bridge name")
	}

	bridgeName := *vnet.Spec.BridgeName
	vni := uint32(100)
	if vnet.Spec.Vni != nil {
		if *vnet.Spec.Vni < 0 || *vnet.Spec.Vni > 16777215 {
			return fmt.Errorf("invalid vni %d", *vnet.Spec.Vni)
		}
		vni = uint32(*vnet.Spec.Vni)
	}

	underlayIf := ""
	if vnet.Spec.UnderlayInterface != nil {
		underlayIf = strings.TrimSpace(*vnet.Spec.UnderlayInterface)
	}
	localIP, err := resolveInterfaceIPv4(underlayIf)
	if err != nil {
		return err
	}

	for _, peerIP := range peers {
		peerIP = strings.TrimSpace(peerIP)
		if peerIP == "" {
			continue
		}

		// トンネル名はブリッジ毎に一意化し、別ネットワーク間の名前衝突を避ける。
		tunnelName := tunnelNameForPeer(bridgeName, peerIP)

		// トンネル存在確認
		exists, err := portExistsOnBridge(bridgeName, tunnelName)
		if err != nil {
			return fmt.Errorf("failed to check existing tunnel %s on %s: %w", tunnelName, bridgeName, err)
		}
		if exists {
			slog.Debug("VXLAN tunnel already exists", "bridge", bridgeName, "tunnel", tunnelName, "peer", peerIP)
		}

		if !exists {
			createCmd := exec.Command("ovs-vsctl", "add-port", bridgeName, tunnelName)
			if output, err := createCmd.CombinedOutput(); err != nil {
				slog.Error("failed to add VXLAN port", "bridge", bridgeName, "tunnel", tunnelName, "peer", peerIP, "err", err, "output", string(output))
				return fmt.Errorf("failed to add tunnel port %s to %s: %w (output=%s)", tunnelName, peerIP, err, strings.TrimSpace(string(output)))
			}
		}

		// トンネル設定（既存トンネルも毎回再同期）
		args := []string{
			"set", "interface", tunnelName,
			"type=vxlan",
			fmt.Sprintf("options:key=%d", vni),
			fmt.Sprintf("options:remote_ip=%s", peerIP),
		}
		if localIP != "" {
			args = append(args, fmt.Sprintf("options:local_ip=%s", localIP))
		}

		setCmd := exec.Command("ovs-vsctl", args...)
		if output, err := setCmd.CombinedOutput(); err != nil {
			slog.Error("failed to configure VXLAN tunnel", "bridge", bridgeName, "tunnel", tunnelName, "peer", peerIP, "err", err, "output", string(output))
			return fmt.Errorf("failed to configure tunnel %s to %s: %w (output=%s)", tunnelName, peerIP, err, strings.TrimSpace(string(output)))
		}

		slog.Info("VXLAN tunnel created", "bridge", bridgeName, "tunnel", tunnelName, "peer", peerIP, "vni", vni, "underlayInterface", underlayIf, "localIP", localIP)
	}

	if err := reconcileSplitHorizonFlows(bridgeName); err != nil {
		return fmt.Errorf("failed to reconcile split-horizon flows on %s: %w", bridgeName, err)
	}

	return nil
}

func tunnelNameForPeer(bridgeName, peerIP string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(bridgeName)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(peerIP)))
	return fmt.Sprintf("vx-%08x", h.Sum32())
}

func portExistsOnBridge(bridgeName, portName string) (bool, error) {
	cmd := exec.Command("ovs-vsctl", "list-ports", bridgeName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("ovs-vsctl list-ports %s failed: %w (output=%s)", bridgeName, err, strings.TrimSpace(string(output)))
	}

	for _, port := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(port) == portName {
			return true, nil
		}
	}

	return false, nil
}

func resolveInterfaceIPv4(interfaceName string) (string, error) {
	name := strings.TrimSpace(interfaceName)
	if name == "" {
		return "", nil
	}

	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", fmt.Errorf("underlay interface %s not found: %w", name, err)
	}
	if iface.Flags&net.FlagUp == 0 {
		return "", fmt.Errorf("underlay interface %s is down", name)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("failed to get addresses on %s: %w", name, err)
	}

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsLoopback() || ip.To4() == nil {
			continue
		}
		return ip.String(), nil
	}

	return "", fmt.Errorf("no ipv4 address found on underlay interface %s", name)
}

// PruneVxlanMesh は残すべきピア以外のトンネルを削除する。
func (o *OVSFabric) PruneVxlanMesh(vnet *api.VirtualNetwork, remainPeers []string) error {
	if vnet == nil || vnet.Spec == nil || vnet.Spec.BridgeName == nil {
		return fmt.Errorf("invalid vnet or missing bridge name")
	}

	bridgeName := *vnet.Spec.BridgeName

	// 現在のトンネル一覧を取得
	cmd := exec.Command("ovs-vsctl", "list-ports", bridgeName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("failed to list ports", "bridge", bridgeName, "err", err)
		return fmt.Errorf("failed to list ports: %w", err)
	}

	currentPorts := strings.Split(strings.TrimSpace(string(output)), "\n")

	// 残すべき tunnel 名リストを作成
	keepTunnels := make(map[string]bool)
	for _, peerIP := range remainPeers {
		peerIP = strings.TrimSpace(peerIP)
		if peerIP != "" {
			tunnelName := tunnelNameForPeer(bridgeName, peerIP)
			keepTunnels[tunnelName] = true
		}
	}

	// vxlan- (旧命名) / vx- (新命名) で始まる不要なトンネルを削除
	for _, port := range currentPorts {
		port = strings.TrimSpace(port)
		isVxlanPort := strings.HasPrefix(port, "vxlan-") || strings.HasPrefix(port, "vx-")
		if isVxlanPort && !keepTunnels[port] {
			delCmd := exec.Command("ovs-vsctl", "del-port", bridgeName, port)
			if output, err := delCmd.CombinedOutput(); err != nil {
				slog.Warn("failed to delete VXLAN tunnel", "bridge", bridgeName, "tunnel", port, "err", err, "output", string(output))
			} else {
				slog.Info("VXLAN tunnel deleted", "bridge", bridgeName, "tunnel", port)
			}
		}
	}

	if err := reconcileSplitHorizonFlows(bridgeName); err != nil {
		return fmt.Errorf("failed to reconcile split-horizon flows on %s: %w", bridgeName, err)
	}

	return nil
}

func reconcileSplitHorizonFlows(bridgeName string) error {
	vxlanPorts, err := listVxlanPortsOnBridge(bridgeName)
	if err != nil {
		return err
	}

	accessPorts, err := listAccessPortsOnBridge(bridgeName)
	if err != nil {
		return err
	}

	// アクセスポートの ofport リストを構築する。
	accessActions := "drop"
	if len(accessPorts) > 0 {
		parts := make([]string, 0, len(accessPorts))
		for _, ap := range accessPorts {
			ofport, err := getInterfaceOfport(ap)
			if err != nil {
				return err
			}
			parts = append(parts, fmt.Sprintf("output:%d", ofport))
		}
		accessActions = strings.Join(parts, ",")
	}

	// 投入すべきフローセットを構築する。
	desired := make([]string, 0, 1+len(vxlanPorts))
	desired = append(desired, fmt.Sprintf("cookie=%s,priority=0,actions=NORMAL", splitHorizonCookie))
	for _, port := range vxlanPorts {
		ofport, err := getInterfaceOfport(port)
		if err != nil {
			return err
		}
		desired = append(desired, fmt.Sprintf("cookie=%s,priority=300,in_port=%d,dl_dst=01:00:00:00:00:00/01:00:00:00:00:00,actions=%s", splitHorizonCookie, ofport, accessActions))
	}

	// 現在のフローセットと比較して変化がなければスキップする。
	current, err := getCurrentSplitHorizonFlows(bridgeName)
	if err == nil && flowSetsEqual(current, desired) {
		slog.Debug("split-horizon flows unchanged, skipping", "bridge", bridgeName)
		return nil
	}

	// bundle トランザクションでアトミックに差し替える（delFlows→addFlow の
	// 間にフローが消えてパケットがドロップするウィンドウをなくす）。
	if err := syncFlowsAtomically(bridgeName, desired); err != nil {
		return err
	}

	slog.Info("split-horizon flows reconciled", "bridge", bridgeName, "vxlanPorts", len(vxlanPorts), "accessPorts", len(accessPorts))
	return nil
}

// syncFlowsAtomically は ovs-ofctl bundle を使い、cookie に紐づく全フローを
// 一括でアトミックに入れ替える。delFlows と addFlow を逐次実行した場合の
// 「フローが空になる瞬間」がなくなり、その間のパケットロスを防ぐ。
func syncFlowsAtomically(bridgeName string, flows []string) error {
	// bundle スクリプトを組み立てる:
	//   del_flows cookie=<cookie>/-1   (既存フロー全削除)
	//   add_flow <flow1>
	//   add_flow <flow2>  ...
	var sb strings.Builder
	fmt.Fprintf(&sb, "del_flows cookie=%s/-1\n", splitHorizonCookie)
	for _, f := range flows {
		fmt.Fprintf(&sb, "add_flow %s\n", f)
	}

	cmd := exec.Command("ovs-ofctl", "-O", "OpenFlow13", "bundle", bridgeName, "-")
	cmd.Stdin = strings.NewReader(sb.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		// bundle が利用できない古い OVS へのフォールバック
		slog.Warn("ovs-ofctl bundle not supported, falling back to non-atomic update", "bridge", bridgeName, "err", err, "output", strings.TrimSpace(string(output)))
		return syncFlowsFallback(bridgeName, flows)
	}
	return nil
}

// syncFlowsFallback は bundle 非対応環境向けの非アトミック差し替え。
func syncFlowsFallback(bridgeName string, flows []string) error {
	if err := delFlows(bridgeName, fmt.Sprintf("cookie=%s/-1", splitHorizonCookie)); err != nil {
		return err
	}
	for _, f := range flows {
		if err := addFlow(bridgeName, f); err != nil {
			return err
		}
	}
	return nil
}

// getCurrentSplitHorizonFlows は現在 OVS に投入されている cookie 一致フローの
// actions 文字列スライスを返す（比較用の簡易表現）。
func getCurrentSplitHorizonFlows(bridgeName string) ([]string, error) {
	cmd := exec.Command("ovs-ofctl", "-O", "OpenFlow13", "dump-flows", bridgeName,
		fmt.Sprintf("cookie=%s/-1", splitHorizonCookie))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("dump-flows failed: %w", err)
	}
	var result []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "OFPST") || strings.HasPrefix(line, "NXST") {
			continue
		}
		// priority と actions 部分だけ抽出してキーとする
		if idx := strings.Index(line, "priority="); idx >= 0 {
			result = append(result, line[idx:])
		}
	}
	return result, nil
}

// flowSetsEqual は2つのフロー記述リストが同じ内容かを順序非依存で比較する。
func flowSetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]int, len(a))
	for _, f := range a {
		set[f]++
	}
	for _, f := range b {
		set[f]--
		if set[f] < 0 {
			return false
		}
	}
	return true
}

func listAccessPortsOnBridge(bridgeName string) ([]string, error) {
	cmd := exec.Command("ovs-vsctl", "list-ports", bridgeName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ovs-vsctl list-ports %s failed: %w (output=%s)", bridgeName, err, strings.TrimSpace(string(output)))
	}

	ports := strings.Split(strings.TrimSpace(string(output)), "\n")
	accessPorts := make([]string, 0, len(ports))
	for _, port := range ports {
		port = strings.TrimSpace(port)
		if port == "" {
			continue
		}
		typeCmd := exec.Command("ovs-vsctl", "get", "interface", port, "type")
		typeOut, typeErr := typeCmd.CombinedOutput()
		if typeErr != nil {
			return nil, fmt.Errorf("ovs-vsctl get interface %s type failed: %w (output=%s)", port, typeErr, strings.TrimSpace(string(typeOut)))
		}
		t := strings.Trim(strings.TrimSpace(string(typeOut)), "\"")
		if t == "" {
			// type が空 = TAP/vnet ポート（VM アクセスポート）
			accessPorts = append(accessPorts, port)
		}
	}
	return accessPorts, nil
}

func listVxlanPortsOnBridge(bridgeName string) ([]string, error) {
	cmd := exec.Command("ovs-vsctl", "list-ports", bridgeName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ovs-vsctl list-ports %s failed: %w (output=%s)", bridgeName, err, strings.TrimSpace(string(output)))
	}

	ports := strings.Split(strings.TrimSpace(string(output)), "\n")
	vxlanPorts := make([]string, 0, len(ports))
	for _, port := range ports {
		port = strings.TrimSpace(port)
		if port == "" {
			continue
		}

		typeCmd := exec.Command("ovs-vsctl", "get", "interface", port, "type")
		typeOut, typeErr := typeCmd.CombinedOutput()
		if typeErr != nil {
			return nil, fmt.Errorf("ovs-vsctl get interface %s type failed: %w (output=%s)", port, typeErr, strings.TrimSpace(string(typeOut)))
		}

		if strings.Trim(strings.TrimSpace(string(typeOut)), "\"") == "vxlan" {
			vxlanPorts = append(vxlanPorts, port)
		}
	}

	return vxlanPorts, nil
}

func getInterfaceOfport(portName string) (int, error) {
	cmd := exec.Command("ovs-vsctl", "get", "interface", portName, "ofport")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("ovs-vsctl get interface %s ofport failed: %w (output=%s)", portName, err, strings.TrimSpace(string(output)))
	}

	text := strings.TrimSpace(string(output))
	text = strings.Trim(text, "\"")
	ofport, convErr := strconv.Atoi(text)
	if convErr != nil || ofport <= 0 {
		return 0, fmt.Errorf("invalid ofport for interface %s: %q", portName, text)
	}

	return ofport, nil
}

func addFlow(bridgeName string, flow string) error {
	cmd := exec.Command("ovs-ofctl", "-O", "OpenFlow13", "add-flow", bridgeName, flow)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ovs-ofctl add-flow %s failed: %w (flow=%s, output=%s)", bridgeName, err, flow, strings.TrimSpace(string(output)))
	}
	return nil
}

func delFlows(bridgeName string, match string) error {
	cmd := exec.Command("ovs-ofctl", "-O", "OpenFlow13", "del-flows", bridgeName, match)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ovs-ofctl del-flows %s failed: %w (match=%s, output=%s)", bridgeName, err, match, strings.TrimSpace(string(output)))
	}
	return nil
}

// DeleteBridge はブリッジをすべて削除する。
func (o *OVSFabric) DeleteBridge(vnet *api.VirtualNetwork) error {
	if vnet == nil || vnet.Spec == nil || vnet.Spec.BridgeName == nil {
		return fmt.Errorf("invalid vnet or missing bridge name")
	}

	bridgeName := *vnet.Spec.BridgeName

	// ブリッジ存在確認
	checkCmd := exec.Command("ovs-vsctl", "br-exists", bridgeName)
	if err := checkCmd.Run(); err != nil {
		// 既に存在しない
		slog.Debug("OVS bridge does not exist, skipping delete", "bridge", bridgeName)
		return nil
	}

	// ブリッジ削除
	cmd := exec.Command("ovs-vsctl", "del-br", bridgeName)
	if output, err := cmd.CombinedOutput(); err != nil {
		slog.Error("failed to delete OVS bridge", "bridge", bridgeName, "err", err, "output", string(output))
		return fmt.Errorf("failed to delete bridge %s: %w", bridgeName, err)
	}

	slog.Info("OVS bridge deleted", "bridge", bridgeName)
	return nil
}

// GetBridgeStatus はブリッジ状態を返す。
func (o *OVSFabric) GetBridgeStatus(vnet *api.VirtualNetwork) (bool, int, error) {
	if vnet == nil || vnet.Spec == nil || vnet.Spec.BridgeName == nil {
		return false, 0, fmt.Errorf("invalid vnet or missing bridge name")
	}

	bridgeName := *vnet.Spec.BridgeName

	// ブリッジ存在確認
	checkCmd := exec.Command("ovs-vsctl", "br-exists", bridgeName)
	if err := checkCmd.Run(); err != nil {
		return false, 0, nil
	}

	// ポート数を取得
	listCmd := exec.Command("ovs-vsctl", "list-ports", bridgeName)
	output, err := listCmd.CombinedOutput()
	if err != nil {
		return true, 0, err
	}

	ports := strings.Split(strings.TrimSpace(string(output)), "\n")
	vxlanCount := 0
	for _, port := range ports {
		if strings.HasPrefix(port, "vxlan-") || strings.HasPrefix(port, "vx-") {
			vxlanCount++
		}
	}

	return true, vxlanCount, nil
}
