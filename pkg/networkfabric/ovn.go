package networkfabric

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/takara9/marmot/api"
)

// OVNFabric は OVN を優先して利用する Fabric 実装。
// 現段階では geneve オーバーレイを OVN で扱い、既存 vxlan は OVS 実装へ委譲する。
type OVNFabric struct {
	ovs *OVSFabric
}

const ovnCommandTimeout = 15 * time.Second

var runOVNNBCTLCommand = runOVNNBCTL
var runOVSVSCTLCommandExec = runOVSVSCTLCommand
var ovnNBCTLLookPath = exec.LookPath
var ovnSBCTLLookPath = exec.LookPath
var enableGeneveOVSTunnelMesh = defaultEnableGeneveOVSTunnelMesh()
var ovnDatabaseConfigMu sync.RWMutex
var ovnNorthboundDBEndpoint string
var ovnSouthboundDBEndpoint string

// NewOVNFabric は OVNFabric インスタンスを生成する。
func NewOVNFabric() *OVNFabric {
	return &OVNFabric{ovs: NewOVSFabric()}
}

func ConfigureOVNCluster(leaderIP, localIP string) error {
	leaderIP = strings.TrimSpace(leaderIP)
	localIP = strings.TrimSpace(localIP)
	if leaderIP == "" {
		return fmt.Errorf("leader IP is required for OVN bootstrap")
	}
	if localIP == "" {
		return fmt.Errorf("local IP is required for OVN bootstrap")
	}

	if localIP == leaderIP {
		if err := ensureLocalOVNCentralTCPListeners(localIP); err != nil {
			return fmt.Errorf("failed to configure OVN central listeners on leader %s: %w", localIP, err)
		}
	}

	nbDB := fmt.Sprintf("tcp:%s:6641", leaderIP)
	sbDB := fmt.Sprintf("tcp:%s:6642", leaderIP)
	setOVNDatabaseEndpoints(nbDB, sbDB)

	if _, err := runOVSVSCTLCommandExec(
		"set", "Open_vSwitch", ".",
		"external_ids:ovn-remote="+sbDB,
		"external_ids:ovn-encap-ip="+localIP,
		"external_ids:ovn-encap-type=geneve",
	); err != nil {
		return fmt.Errorf("failed to configure local OVN chassis bootstrap: %w", err)
	}

	return nil
}

func ensureLocalOVNCentralTCPListeners(bindIP string) error {
	bindIP = strings.TrimSpace(bindIP)
	if bindIP == "" {
		return fmt.Errorf("bind ip is required")
	}

	if _, err := runOVNNBCTLLocal("set-connection", "ptcp:6641:"+bindIP); err != nil {
		return fmt.Errorf("ovn-nbctl set-connection failed: %w", err)
	}
	if _, err := runOVNSBCTLLocal("set-connection", "ptcp:6642:"+bindIP); err != nil {
		return fmt.Errorf("ovn-sbctl set-connection failed: %w", err)
	}
	return nil
}

func (o *OVNFabric) EnsureBridge(vnet *api.VirtualNetwork) error {
	if isGeneveOverlay(vnet) {
		if !ovnCommandsAvailable() {
			slog.Warn("OVN commands are not available; falling back to OVS bridge ensure", "network", networkName(vnet))
		}
		// 現行 libvirt 連携では bridge 実体が必要なため、geneve でも一旦 OVS bridge を維持する。
	}
	return o.ovs.EnsureBridge(vnet)
}

func (o *OVNFabric) EnsureOverlayMesh(vnet *api.VirtualNetwork, peers []string) error {
	if isGeneveOverlay(vnet) {
		if ovnCommandsAvailable() {
			lsName, err := ensureGeneveLogicalSwitch(vnet)
			if err != nil {
				if !enableGeneveOVSTunnelMesh {
					return err
				}
				slog.Warn("failed to ensure OVN logical switch for geneve overlay; continue with OVS tunnel mesh", "network", networkName(vnet), "err", err)
			} else if err := syncGeneveLogicalPorts(vnet, lsName); err != nil {
				if !enableGeneveOVSTunnelMesh {
					return err
				}
				slog.Warn("failed to sync OVN logical switch ports for geneve overlay; continue with OVS tunnel mesh", "network", networkName(vnet), "logicalSwitch", lsName, "err", err)
			} else if !enableGeneveOVSTunnelMesh {
				if err := pruneLegacyOVSTunnels(vnet); err != nil {
					return fmt.Errorf("failed to prune legacy OVS overlay tunnels for OVN-only geneve mode: %w", err)
				}
				return nil
			}
		} else {
			slog.Warn("OVN commands are not available; using OVS tunnel mesh for geneve overlay", "network", networkName(vnet))
			if !enableGeneveOVSTunnelMesh {
				return fmt.Errorf("ovn-nbctl/ovn-sbctl are required for geneve overlay")
			}
		}

		return o.ovs.EnsureOverlayMesh(vnet, peers)
	}

	return o.ovs.EnsureOverlayMesh(vnet, peers)
}

func (o *OVNFabric) PruneOverlayMesh(vnet *api.VirtualNetwork, remainPeers []string) error {
	if isGeneveOverlay(vnet) {
		if ovnCommandsAvailable() {
			if !enableGeneveOVSTunnelMesh {
				if err := pruneLegacyOVSTunnels(vnet); err != nil {
					return fmt.Errorf("failed to prune legacy OVS overlay tunnels for OVN-only geneve mode: %w", err)
				}
				// OVN dataplaneではリモートノードの論理ポートを誤削除しないため、
				// ノード単位の logical_switch_port prune は実施しない。
				return nil
			}
		} else {
			slog.Warn("OVN commands are not available during geneve prune; using OVS tunnel prune", "network", networkName(vnet))
			if !enableGeneveOVSTunnelMesh {
				return fmt.Errorf("ovn-nbctl/ovn-sbctl are required for geneve overlay")
			}
		}
		return o.ovs.PruneOverlayMesh(vnet, remainPeers)
	}

	return o.ovs.PruneOverlayMesh(vnet, remainPeers)
}

func (o *OVNFabric) DeleteBridge(vnet *api.VirtualNetwork) error {
	if isGeneveOverlay(vnet) && ovnCommandsAvailable() {
		if err := deleteGeneveLogicalSwitch(vnet); err != nil {
			slog.Warn("failed to delete OVN logical switch for geneve overlay; continue with OVS bridge delete", "network", networkName(vnet), "err", err)
		}
	}
	return o.ovs.DeleteBridge(vnet)
}

func (o *OVNFabric) GetBridgeStatus(vnet *api.VirtualNetwork) (bool, int, error) {
	return o.ovs.GetBridgeStatus(vnet)
}

func isGeneveOverlay(vnet *api.VirtualNetwork) bool {
	if vnet == nil || vnet.Spec.OverlayMode == nil {
		return false
	}
	return strings.EqualFold(string(*vnet.Spec.OverlayMode), string(api.Geneve))
}

func ovnCommandsAvailable() bool {
	if _, err := ovnNBCTLLookPath("ovn-nbctl"); err != nil {
		return false
	}
	if _, err := ovnSBCTLLookPath("ovn-sbctl"); err != nil {
		return false
	}
	return true
}

func ensureGeneveLogicalSwitch(vnet *api.VirtualNetwork) (string, error) {
	lsName := logicalSwitchName(vnet)
	if lsName == "" {
		return "", fmt.Errorf("unable to determine OVN logical switch name")
	}

	if _, err := runOVNNBCTLCommand("--may-exist", "ls-add", lsName); err != nil {
		return "", fmt.Errorf("failed to ensure OVN logical switch %s: %w", lsName, err)
	}

	// 運用時の追跡のため、Marmot側識別子を external_ids に保持する。
	if id := networkID(vnet); id != "" {
		if _, err := runOVNNBCTLCommand("set", "logical_switch", lsName, "external_ids:marmot_network_id="+id); err != nil {
			return "", fmt.Errorf("failed to set external_ids on OVN logical switch %s: %w", lsName, err)
		}
	}

	if bridge := bridgeName(vnet); bridge != "" {
		if _, err := runOVNNBCTLCommand("set", "logical_switch", lsName, "external_ids:marmot_bridge_name="+bridge); err != nil {
			return "", fmt.Errorf("failed to set bridge external_id on OVN logical switch %s: %w", lsName, err)
		}
	}

	return lsName, nil
}

func deleteGeneveLogicalSwitch(vnet *api.VirtualNetwork) error {
	lsName := logicalSwitchName(vnet)
	if lsName == "" {
		return nil
	}
	if _, err := runOVNNBCTLCommand("--if-exists", "ls-del", lsName); err != nil {
		return fmt.Errorf("failed to delete OVN logical switch %s: %w", lsName, err)
	}
	return nil
}

func syncGeneveLogicalPorts(vnet *api.VirtualNetwork, lsName string) error {
	bridge := bridgeName(vnet)
	if bridge == "" {
		return fmt.Errorf("bridge name is required for OVN geneve port sync")
	}

	ifaceNames, err := listOVSBridgePorts(bridge)
	if err != nil {
		return err
	}

	nodeName, _ := os.Hostname()
	nodeName = sanitizeOVNToken(nodeName)
	if nodeName == "" {
		nodeName = "unknown-node"
	}

	for _, ifaceName := range ifaceNames {
		if skipOVNPortSyncInterface(ifaceName, bridge) {
			continue
		}

		portName := genevePortNameForInterface(lsName, nodeName, ifaceName)
		if _, err := runOVSVSCTLCommandExec("set", "interface", ifaceName, "external_ids:iface-id="+portName); err != nil {
			return fmt.Errorf("failed to set iface-id on interface %s: %w", ifaceName, err)
		}

		if _, err := runOVNNBCTLCommand("--may-exist", "lsp-add", lsName, portName); err != nil {
			return fmt.Errorf("failed to ensure OVN logical switch port %s on %s: %w", portName, lsName, err)
		}
		if _, err := runOVNNBCTLCommand("set", "logical_switch_port", portName, "addresses=\"unknown\""); err != nil {
			return fmt.Errorf("failed to set addresses on OVN logical switch port %s: %w", portName, err)
		}
		if id := networkID(vnet); id != "" {
			if _, err := runOVNNBCTLCommand("set", "logical_switch_port", portName, "external_ids:marmot_network_id="+id); err != nil {
				return fmt.Errorf("failed to set network external_id on OVN logical switch port %s: %w", portName, err)
			}
		}
		if _, err := runOVNNBCTLCommand("set", "logical_switch_port", portName, "external_ids:marmot_managed=true"); err != nil {
			return fmt.Errorf("failed to set managed external_id on OVN logical switch port %s: %w", portName, err)
		}
		if _, err := runOVNNBCTLCommand("set", "logical_switch_port", portName, "external_ids:marmot_node_name="+nodeName); err != nil {
			return fmt.Errorf("failed to set node external_id on OVN logical switch port %s: %w", portName, err)
		}
		if _, err := runOVNNBCTLCommand("set", "logical_switch_port", portName, "external_ids:marmot_ovs_interface="+ifaceName); err != nil {
			return fmt.Errorf("failed to set interface external_id on OVN logical switch port %s: %w", portName, err)
		}
	}

	return nil
}

func pruneGeneveLogicalPorts(vnet *api.VirtualNetwork, lsName string, remainPeers []string) error {
	currentPorts, err := listLogicalSwitchPorts(lsName)
	if err != nil {
		return err
	}

	keep := map[string]struct{}{}
	for _, name := range desiredGenevePortNames(lsName, remainPeers) {
		keep[name] = struct{}{}
	}

	keptManagedPorts := make([]string, 0, len(currentPorts))
	for _, port := range currentPorts {
		if !strings.HasPrefix(port, "marmot-peer-") {
			continue
		}
		if _, ok := keep[port]; ok {
			keptManagedPorts = append(keptManagedPorts, port)
			continue
		}
		if _, err := runOVNNBCTLCommand("--if-exists", "lsp-del", port); err != nil {
			return fmt.Errorf("failed to delete OVN logical switch port %s: %w", port, err)
		}
	}

	if id := networkID(vnet); id != "" {
		for _, port := range keptManagedPorts {
			if _, err := runOVNNBCTLCommand("set", "logical_switch_port", port, "external_ids:marmot_network_id="+id); err != nil {
				return fmt.Errorf("failed to refresh network external_id on OVN logical switch port %s: %w", port, err)
			}
		}
	}

	return nil
}

func listLogicalSwitchPorts(lsName string) ([]string, error) {
	output, err := runOVNNBCTLCommand("lsp-list", lsName)
	if err != nil {
		// スイッチが存在しない場合は空扱いにする。
		if strings.Contains(err.Error(), "does not exist") {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list OVN logical switch ports for %s: %w", lsName, err)
	}

	ports := []string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		ports = append(ports, fields[0])
	}
	return ports, nil
}

func desiredGenevePortNames(lsName string, peers []string) []string {
	unique := map[string]struct{}{}
	for _, peer := range peers {
		peer = strings.TrimSpace(peer)
		if peer == "" {
			continue
		}
		unique[genevePortNameForPeer(lsName, peer)] = struct{}{}
	}

	result := make([]string, 0, len(unique))
	for name := range unique {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func genevePortNameForInterface(lsName, nodeName, ifaceName string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(lsName)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(nodeName)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(ifaceName)))
	return fmt.Sprintf("marmot-vm-%08x", h.Sum32())
}

func genevePortNameForPeer(lsName, peer string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(lsName)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(peer)))
	return fmt.Sprintf("marmot-peer-%08x", h.Sum32())
}

func listOVSBridgePorts(bridgeName string) ([]string, error) {
	output, err := runOVSVSCTLCommandExec("list-ports", bridgeName)
	if err != nil {
		if strings.Contains(err.Error(), "no bridge named") {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list OVS bridge ports on %s: %w", bridgeName, err)
	}

	ports := []string{}
	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		ports = append(ports, name)
	}
	return ports, nil
}

func skipOVNPortSyncInterface(ifaceName, bridgeName string) bool {
	name := strings.TrimSpace(strings.ToLower(ifaceName))
	if name == "" {
		return true
	}
	if strings.EqualFold(ifaceName, bridgeName) {
		return true
	}
	if strings.HasPrefix(name, "vx-") || strings.HasPrefix(name, "gn-") {
		return true
	}
	if strings.HasPrefix(name, "patch-") || strings.HasPrefix(name, "phy-") {
		return true
	}
	if strings.HasPrefix(name, "qvo") || strings.HasPrefix(name, "qvb") || strings.HasPrefix(name, "qbr") {
		return true
	}
	return false
}

func pruneLegacyOVSTunnels(vnet *api.VirtualNetwork) error {
	bridge := bridgeName(vnet)
	if bridge == "" {
		return nil
	}

	ports, err := listOVSBridgePorts(bridge)
	if err != nil {
		return err
	}

	for _, port := range ports {
		name := strings.TrimSpace(strings.ToLower(port))
		if !(strings.HasPrefix(name, "vx-") || strings.HasPrefix(name, "gn-")) {
			continue
		}
		if _, err := runOVSVSCTLCommandExec("--if-exists", "del-port", bridge, port); err != nil {
			return fmt.Errorf("failed to delete legacy OVS tunnel port %s on %s: %w", port, bridge, err)
		}
	}

	return nil
}

func sanitizeOVNToken(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	v = ovnLogicalSwitchSanitizer.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	return v
}

func runOVSVSCTLCommand(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ovnCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ovs-vsctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ovs-vsctl timeout: args=%v", args)
		}
		return "", fmt.Errorf("ovs-vsctl failed: args=%v output=%s err=%w", args, strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func defaultEnableGeneveOVSTunnelMesh() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("MARMOT_ENABLE_GENEVE_OVS_MESH")))
	switch v {
	case "0", "false", "no", "off":
		return false
	case "1", "true", "yes", "on", "":
		return true
	default:
		// Unknown values keep backward-compatible default (enabled).
		return true
	}
}

func runOVNNBCTL(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ovnCommandTimeout)
	defer cancel()

	cmdArgs, usedConfiguredDB := withOVNNBDatabase(args)
	cmd := exec.CommandContext(ctx, "ovn-nbctl", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil && usedConfiguredDB && isOVNDBConnectionError(string(output)) {
		// Remote NBDB endpoint can be temporarily unavailable or not exposed via TCP.
		// Retry once with ovn-nbctl default connection settings (typically local unix socket).
		cmd = exec.CommandContext(ctx, "ovn-nbctl", args...)
		output, err = cmd.CombinedOutput()
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ovn-nbctl timeout: args=%v", args)
		}
		return "", fmt.Errorf("ovn-nbctl failed: args=%v output=%s err=%w", args, strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func runOVNNBCTLLocal(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ovnCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ovn-nbctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ovn-nbctl timeout: args=%v", args)
		}
		return "", fmt.Errorf("ovn-nbctl failed: args=%v output=%s err=%w", args, strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func runOVNSBCTLLocal(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ovnCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ovn-sbctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ovn-sbctl timeout: args=%v", args)
		}
		return "", fmt.Errorf("ovn-sbctl failed: args=%v output=%s err=%w", args, strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func setOVNDatabaseEndpoints(nbDB, sbDB string) {
	ovnDatabaseConfigMu.Lock()
	defer ovnDatabaseConfigMu.Unlock()
	ovnNorthboundDBEndpoint = strings.TrimSpace(nbDB)
	ovnSouthboundDBEndpoint = strings.TrimSpace(sbDB)
}

func withOVNNBDatabase(args []string) ([]string, bool) {
	ovnDatabaseConfigMu.RLock()
	nbDB := ovnNorthboundDBEndpoint
	ovnDatabaseConfigMu.RUnlock()
	if nbDB == "" {
		return args, false
	}
	withDB := make([]string, 0, len(args)+1)
	withDB = append(withDB, "--db="+nbDB)
	withDB = append(withDB, args...)
	return withDB, true
}

func isOVNDBConnectionError(output string) bool {
	lower := strings.ToLower(strings.TrimSpace(output))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "database connection failed") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "failed to connect")
}

var ovnLogicalSwitchSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func logicalSwitchName(vnet *api.VirtualNetwork) string {
	key := networkName(vnet)
	if key == "" {
		key = networkID(vnet)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	safe := ovnLogicalSwitchSanitizer.ReplaceAllString(key, "-")
	return "marmot-net-" + safe
}

func networkID(vnet *api.VirtualNetwork) string {
	if vnet == nil {
		return ""
	}
	return strings.TrimSpace(vnet.Metadata.Id)
}

func bridgeName(vnet *api.VirtualNetwork) string {
	if vnet == nil || vnet.Spec.BridgeName == nil {
		return ""
	}
	return strings.TrimSpace(*vnet.Spec.BridgeName)
}

func networkName(vnet *api.VirtualNetwork) string {
	if vnet == nil {
		return ""
	}
	return strings.TrimSpace(vnet.Metadata.Name)
}
