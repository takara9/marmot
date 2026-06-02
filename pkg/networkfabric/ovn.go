package networkfabric

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os/exec"
	"regexp"
	"sort"
	"strings"
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
var runOVNSBCTLCommand = runOVNSBCTL
var runOVSVSCTLCommand = runOVSVSCTL
var ovnNBCTLLookPath = exec.LookPath
var ovnSBCTLLookPath = exec.LookPath
var enableGeneveOVSTunnelMesh = true

// NewOVNFabric は OVNFabric インスタンスを生成する。
func NewOVNFabric() *OVNFabric {
	return &OVNFabric{ovs: NewOVSFabric()}
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
		if !ovnCommandsAvailable() {
			return fmt.Errorf("ovn-nbctl/ovn-sbctl are required for geneve overlay")
		}

		if err := ensureGeneveOVNReadiness(peers); err != nil {
			return err
		}

		lsName, err := ensureGeneveLogicalSwitch(vnet)
		if err != nil {
			return err
		}
		if err := syncGeneveLogicalPorts(vnet, lsName, peers); err != nil {
			return err
		}
		if enableGeneveOVSTunnelMesh {
			return o.ovs.EnsureOverlayMesh(vnet, peers)
		}
		return nil
	}

	return o.ovs.EnsureOverlayMesh(vnet, peers)
}

func (o *OVNFabric) PruneOverlayMesh(vnet *api.VirtualNetwork, remainPeers []string) error {
	if isGeneveOverlay(vnet) {
		if !ovnCommandsAvailable() {
			return fmt.Errorf("ovn-nbctl/ovn-sbctl are required for geneve overlay")
		}

		lsName := logicalSwitchName(vnet)
		if lsName == "" {
			return nil
		}
		if err := pruneGeneveLogicalPorts(vnet, lsName, remainPeers); err != nil {
			return err
		}
		if enableGeneveOVSTunnelMesh {
			return o.ovs.PruneOverlayMesh(vnet, remainPeers)
		}
		return nil
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

func ensureGeneveOVNReadiness(peers []string) error {
	output, err := runOVNSBCTLCommand("--format=csv", "--data=bare", "--no-headings", "--columns=type", "list", "encap")
	if err != nil {
		return fmt.Errorf("failed to query OVN southbound encap state: %w", err)
	}

	hasGeneveEncap := false
	for _, line := range strings.Split(output, "\n") {
		if strings.EqualFold(strings.TrimSpace(line), "geneve") {
			hasGeneveEncap = true
			break
		}
	}

	if !hasGeneveEncap {
		if len(peers) == 0 {
			return fmt.Errorf("OVN southbound has no geneve encap entries")
		}
		return fmt.Errorf("OVN southbound has no geneve encap entries; ensure ovn-controller is connected and OVS external_ids include ovn-remote/ovn-encap-ip/ovn-encap-type=geneve")
	}

	return nil
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

func syncGeneveLogicalPorts(vnet *api.VirtualNetwork, lsName string, peers []string) error {
	desired := desiredGenevePortNames(lsName, peers)
	for _, portName := range desired {
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
	}

	return pruneGeneveLogicalPorts(vnet, lsName, peers)
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

func genevePortNameForPeer(lsName, peer string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(lsName)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(peer)))
	return fmt.Sprintf("marmot-peer-%08x", h.Sum32())
}

func runOVNNBCTL(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ovnCommandTimeout)
	defer cancel()

	resolvedArgs := appendOVNDBTargetArgs(args, ovnNorthboundDBTarget())
	cmd := exec.CommandContext(ctx, "ovn-nbctl", resolvedArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ovn-nbctl timeout: args=%v", resolvedArgs)
		}
		return "", fmt.Errorf("ovn-nbctl failed: args=%v output=%s err=%w", resolvedArgs, strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func runOVNSBCTL(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ovnCommandTimeout)
	defer cancel()

	resolvedArgs := appendOVNDBTargetArgs(args, ovnSouthboundDBTarget())
	cmd := exec.CommandContext(ctx, "ovn-sbctl", resolvedArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ovn-sbctl timeout: args=%v", resolvedArgs)
		}
		return "", fmt.Errorf("ovn-sbctl failed: args=%v output=%s err=%w", resolvedArgs, strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func appendOVNDBTargetArgs(args []string, dbTarget string) []string {
	if strings.TrimSpace(dbTarget) == "" {
		return args
	}
	resolved := make([]string, 0, len(args)+1)
	resolved = append(resolved, "--db="+strings.TrimSpace(dbTarget))
	resolved = append(resolved, args...)
	return resolved
}

func ovnSouthboundDBTarget() string {
	remote, ok := ovsExternalIDValue("ovn-remote")
	if !ok {
		return ""
	}
	return normalizeOVNDBTarget(remote)
}

func ovnNorthboundDBTarget() string {
	southbound, ok := ovsExternalIDValue("ovn-remote")
	if !ok {
		return ""
	}
	target := normalizeOVNDBTarget(southbound)
	if target == "" {
		return ""
	}
	if strings.HasSuffix(target, ":6642") {
		return strings.TrimSuffix(target, ":6642") + ":6641"
	}
	return target
}

func normalizeOVNDBTarget(target string) string {
	target = strings.TrimSpace(target)
	target = strings.Trim(target, "\"")
	if target == "" || target == "[]" || target == "{}" {
		return ""
	}
	return target
}

func ovsExternalIDValue(key string) (string, bool) {
	k := strings.TrimSpace(key)
	if k == "" {
		return "", false
	}
	output, err := runOVSVSCTLCommand("get", "open_vswitch", ".", "external_ids:"+k)
	if err != nil {
		return "", false
	}
	value := normalizeOVNDBTarget(output)
	if value == "" {
		return "", false
	}
	return value, true
}

func runOVSVSCTL(args ...string) (string, error) {
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

var ovnLogicalSwitchSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func logicalSwitchName(vnet *api.VirtualNetwork) string {
	id := networkID(vnet)
	if id == "" {
		id = networkName(vnet)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	safe := ovnLogicalSwitchSanitizer.ReplaceAllString(id, "-")
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
