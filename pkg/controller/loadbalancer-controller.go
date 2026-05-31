package controller

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/networkfabric"
	"github.com/takara9/marmot/pkg/util"
)

const (
	LOAD_BALANCER_CONTROLLER_INTERVAL = 15 * time.Second
	loadBalancerBackendModeAuto       = "auto"
	loadBalancerBackendModeManual     = "manual"
	loadBalancerAutoBackendLabelKey   = "lb-enabled"
	loadBalancerAutoBackendLabelValue = "true"
	loadBalancerLabelLogicalSwitch    = "logicalSwitchName"
	loadBalancerLabelOVNName          = "ovnLoadBalancerName"
	loadBalancerLabelResolvedBackends = "resolvedBackends"
)

var (
	loadBalancerLogicalSwitchSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	newLoadBalancerFabric              = func() networkfabric.LoadBalancerFabric { return networkfabric.NewOVNFabric() }
)

type loadBalancerDesiredState struct {
	ID                string
	LogicalSwitchName string
	ResolvedVIP       string
	ResolvedBackends  []string
	NormalizedPorts   []string
	VIPs              map[string]string
	ConfigHash        string
}

type loadBalancerResolveOutcome struct {
	desired        *loadBalancerDesiredState
	requeueMessage string
	fatalErr       error
}

// StartLoadBalancerController starts controller loop for load balancer resources.
func StartLoadBalancerController(node string, etcdUrl string) (*controller, error) {
	var c controller
	var err error

	c.deletionDelay = 15 * time.Second
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	ticker := time.NewTicker(LOAD_BALANCER_CONTROLLER_INTERVAL)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.loadBalancerControllerLoop()
			case <-c.stopChan:
				slog.Info("ロードバランサーコントローラー停止")
				return
			}
		}
	}()

	return &c, nil
}

func (c *controller) loadBalancerControllerLoop() {
	slog.Debug("ロードバランサーコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))

	items, err := c.db.GetLoadBalancers()
	if err != nil {
		slog.Error("GetLoadBalancers() failed", "err", err)
		return
	}

	fabric := newLoadBalancerFabric()

	for _, item := range items {
		id := strings.TrimSpace(api.LoadBalancerID(item))
		if id == "" {
			slog.Warn("load balancer id is empty, skip")
			continue
		}

		if !loadBalancerHasAssignedNode(item) {
			if err := c.assignLoadBalancerNode(id); err != nil {
				slog.Warn("failed to assign node to load balancer", "id", id, "err", err)
			}
			continue
		}
		if ok, assignedNode, reason := evaluateNodeAssignment(&item.Metadata, c.marmot.NodeName); !ok {
			slog.Debug("別ノード割当のロードバランサーをスキップ", "id", id, "name", item.Metadata.Name, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			continue
		}

		if item.Status == nil {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_PENDING, "")
			continue
		}

		if item.Status.DeletionTimeStamp != nil && time.Since(*item.Status.DeletionTimeStamp) > c.deletionDelay {
			if item.Status.StatusCode != db.LOAD_BALANCER_DELETING {
				_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_DELETING, "")
				continue
			}
		}

		switch item.Status.StatusCode {
		case db.LOAD_BALANCER_PENDING:
			c.reconcileLoadBalancerPending(item)
		case db.LOAD_BALANCER_PROVISIONING:
			c.reconcileLoadBalancerProvisioning(item, fabric)
		case db.LOAD_BALANCER_ACTIVE:
			c.reconcileLoadBalancerActive(item)
		case db.LOAD_BALANCER_DELETING:
			c.reconcileLoadBalancerDeleting(item, fabric)
		case db.LOAD_BALANCER_FAILED:
			slog.Debug("FAILED 状態のロードバランサーを検出", "id", id)
		default:
			slog.Warn("不明なロードバランサー状態", "id", id, "statusCode", item.Status.StatusCode)
		}
	}
}

func (c *controller) reconcileLoadBalancerPending(lb api.LoadBalancer) {
	id := api.LoadBalancerID(lb)
	outcome := c.resolveLoadBalancerDesiredState(lb)
	if outcome.fatalErr != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_FAILED, outcome.fatalErr.Error())
		return
	}
	if outcome.requeueMessage != "" {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_PROVISIONING, outcome.requeueMessage)
		return
	}
	_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_PROVISIONING, "load balancer desired state resolved")
}

func (c *controller) reconcileLoadBalancerProvisioning(lb api.LoadBalancer, fabric networkfabric.LoadBalancerFabric) {
	id := api.LoadBalancerID(lb)
	outcome := c.resolveLoadBalancerDesiredState(lb)
	if outcome.fatalErr != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_FAILED, outcome.fatalErr.Error())
		return
	}
	if outcome.requeueMessage != "" {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_PROVISIONING, outcome.requeueMessage)
		return
	}
	if outcome.desired == nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_PROVISIONING, "load balancer desired state is empty")
		return
	}
	if err := c.applyLoadBalancerDesiredState(lb, *outcome.desired, fabric); err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_FAILED, err.Error())
		return
	}
	_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_ACTIVE, "")
}

func (c *controller) reconcileLoadBalancerActive(lb api.LoadBalancer) {
	id := api.LoadBalancerID(lb)
	outcome := c.resolveLoadBalancerDesiredState(lb)
	if outcome.fatalErr != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_FAILED, outcome.fatalErr.Error())
		return
	}
	if outcome.requeueMessage != "" {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_PROVISIONING, outcome.requeueMessage)
		return
	}
	if outcome.desired == nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_PROVISIONING, "load balancer desired state is empty")
		return
	}
	if loadBalancerAppliedConfigHash(lb) != outcome.desired.ConfigHash {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(id, db.LOAD_BALANCER_PROVISIONING, "load balancer configuration drift detected")
	}
}

func (c *controller) reconcileLoadBalancerDeleting(lb api.LoadBalancer, fabric networkfabric.LoadBalancerFabric) {
	id := api.LoadBalancerID(lb)
	lsName := loadBalancerLogicalSwitchNameFromLabels(lb)
	if lsName == "" {
		if vnet, err := c.db.GetVirtualNetworkByName(strings.TrimSpace(lb.Spec.InternalVirtualNetwork)); err == nil {
			lsName = loadBalancerLogicalSwitchName(vnet)
		}
	}
	if err := fabric.DeleteLoadBalancer(id, lsName); err != nil {
		slog.Warn("DeleteLoadBalancer() failed", "id", id, "logicalSwitch", lsName, "err", err)
	}

	hostname := strings.TrimSpace(lb.Metadata.Name)
	subdomain := strings.TrimSpace(lb.Spec.InternalVirtualNetwork)
	if hostname != "" && subdomain != "" {
		if err := c.db.DeleteDnsEntryByName(hostname, subdomain); err != nil && !errors.Is(err, db.ErrNotFound) {
			slog.Warn("DeleteDnsEntryByName() failed", "id", id, "hostname", hostname, "subdomain", subdomain, "err", err)
		}
	}

	if lb.Spec.VirtualIpAddress == nil || strings.TrimSpace(*lb.Spec.VirtualIpAddress) == "" {
		_ = c.releaseLoadBalancerAllocatedVIP(lb)
	}

	if err := c.db.DeleteLoadBalancerById(id); err != nil {
		slog.Warn("DeleteLoadBalancerById() failed", "id", id, "err", err)
	}
}

func (c *controller) resolveLoadBalancerDesiredState(lb api.LoadBalancer) loadBalancerResolveOutcome {
	id := strings.TrimSpace(api.LoadBalancerID(lb))
	if id == "" {
		return loadBalancerResolveOutcome{fatalErr: fmt.Errorf("load balancer id is empty")}
	}
	if strings.TrimSpace(lb.Spec.InternalVirtualNetwork) == "" {
		return loadBalancerResolveOutcome{fatalErr: fmt.Errorf("spec.internalVirtualNetwork is required")}
	}
	if lb.Spec.BindPublicIpAddress != nil && strings.TrimSpace(*lb.Spec.BindPublicIpAddress) != "" {
		return loadBalancerResolveOutcome{fatalErr: fmt.Errorf("spec.bindPublicIpAddress is out of scope for this release")}
	}

	vnet, err := c.db.GetVirtualNetworkByName(strings.TrimSpace(lb.Spec.InternalVirtualNetwork))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return loadBalancerResolveOutcome{fatalErr: fmt.Errorf("internalVirtualNetwork %q is not found", strings.TrimSpace(lb.Spec.InternalVirtualNetwork))}
		}
		return loadBalancerResolveOutcome{fatalErr: err}
	}

	normalizedPorts, err := marmotd.NormalizeServerPorts(lb.Spec.ServerPorts)
	if err != nil {
		return loadBalancerResolveOutcome{fatalErr: err}
	}
	if len(normalizedPorts) == 0 {
		return loadBalancerResolveOutcome{fatalErr: fmt.Errorf("spec.serverPorts is required")}
	}

	backendMode := resolveLoadBalancerBackendMode(lb.Spec.BackendMode)
	if backendMode != loadBalancerBackendModeAuto && backendMode != loadBalancerBackendModeManual {
		return loadBalancerResolveOutcome{fatalErr: fmt.Errorf("spec.backendMode must be manual or auto")}
	}

	var backends []string
	if backendMode == loadBalancerBackendModeManual {
		resolved, recoverable, resolveErr := c.resolveManualLoadBalancerBackends(lb)
		if resolveErr != nil {
			if recoverable {
				return loadBalancerResolveOutcome{requeueMessage: resolveErr.Error()}
			}
			return loadBalancerResolveOutcome{fatalErr: resolveErr}
		}
		backends = resolved
	} else {
		if len(lb.Spec.InternalServers) > 0 {
			return loadBalancerResolveOutcome{fatalErr: fmt.Errorf("spec.internalServers is forbidden when backendMode=auto")}
		}
		resolved, err := c.resolveAutoBackendsOnNetwork(lb.Spec.InternalVirtualNetwork, loadBalancerAutoBackendLabelKey, loadBalancerAutoBackendLabelValue)
		if err != nil {
			return loadBalancerResolveOutcome{fatalErr: err}
		}
		backends = resolved
	}

	if len(backends) == 0 {
		return loadBalancerResolveOutcome{requeueMessage: "no backend servers resolved"}
	}

	resolvedVIP, err := c.resolveLoadBalancerVIP(lb, vnet)
	if err != nil {
		return loadBalancerResolveOutcome{fatalErr: err}
	}
	for _, backend := range backends {
		if backend == resolvedVIP {
			return loadBalancerResolveOutcome{fatalErr: fmt.Errorf("resolved virtual IP %q conflicts with backend IP", resolvedVIP)}
		}
	}

	logicalSwitchName := loadBalancerLogicalSwitchName(vnet)
	if logicalSwitchName == "" {
		return loadBalancerResolveOutcome{fatalErr: fmt.Errorf("failed to determine logical switch name")}
	}

	vipMap := make(map[string]string, len(normalizedPorts))
	for _, spec := range normalizedPorts {
		parts := strings.Split(spec, "/")
		if len(parts) != 2 {
			return loadBalancerResolveOutcome{fatalErr: fmt.Errorf("invalid normalized serverPort %q", spec)}
		}
		vipMap[fmt.Sprintf("%s:%s", resolvedVIP, parts[0])] = strings.Join(withPort(backends, parts[0]), ",")
	}

	desired := &loadBalancerDesiredState{
		ID:                id,
		LogicalSwitchName: logicalSwitchName,
		ResolvedVIP:       resolvedVIP,
		ResolvedBackends:  append([]string(nil), backends...),
		NormalizedPorts:   append([]string(nil), normalizedPorts...),
		VIPs:              vipMap,
		ConfigHash:        desiredLoadBalancerConfigHash(lb, backendMode, resolvedVIP, normalizedPorts, backends),
	}

	return loadBalancerResolveOutcome{desired: desired}
}

func (c *controller) applyLoadBalancerDesiredState(lb api.LoadBalancer, desired loadBalancerDesiredState, fabric networkfabric.LoadBalancerFabric) error {
	externalIDs := map[string]string{}
	if networkID := strings.TrimSpace(lb.Spec.InternalVirtualNetwork); networkID != "" {
		externalIDs["marmot_network_name"] = networkID
	}

	ovnName, err := fabric.EnsureLoadBalancer(networkfabric.OVNLoadBalancerSpec{
		LoadBalancerID:    desired.ID,
		LogicalSwitchName: desired.LogicalSwitchName,
		VIPs:              desired.VIPs,
		ExternalIDs:       externalIDs,
	})
	if err != nil {
		return err
	}

	hostname := strings.TrimSpace(lb.Metadata.Name)
	subdomain := strings.TrimSpace(lb.Spec.InternalVirtualNetwork)
	if hostname != "" && subdomain != "" {
		if err := c.db.PutDnsEntry(hostname, subdomain, desired.ResolvedVIP); err != nil {
			return err
		}
	}

	return c.updateLoadBalancerLabels(desired.ID, func(labels map[string]interface{}) {
		db.SetLoadBalancerResolvedVIP(labels, desired.ResolvedVIP)
		db.SetLoadBalancerAppliedConfigHash(labels, desired.ConfigHash)
		labels[loadBalancerLabelLogicalSwitch] = desired.LogicalSwitchName
		labels[loadBalancerLabelOVNName] = ovnName
		labels[loadBalancerLabelResolvedBackends] = strings.Join(desired.ResolvedBackends, ",")
	})
}

func (c *controller) resolveManualLoadBalancerBackends(lb api.LoadBalancer) ([]string, bool, error) {
	if len(lb.Spec.InternalServers) == 0 {
		return nil, false, fmt.Errorf("spec.internalServers is required when backendMode=manual")
	}

	resolved := make([]string, 0, len(lb.Spec.InternalServers))
	seen := map[string]struct{}{}

	for _, rawName := range lb.Spec.InternalServers {
		serverName := strings.TrimSpace(rawName)
		if serverName == "" {
			continue
		}
		server, err := c.findServerByName(serverName)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return nil, false, fmt.Errorf("manual backend server %q is not found", serverName)
			}
			return nil, false, err
		}
		if server.Status == nil || server.Status.StatusCode != db.SERVER_RUNNING {
			return nil, true, fmt.Errorf("manual backend server %q is not running", serverName)
		}
		ip, ok := serverAddressOnNetwork(server, lb.Spec.InternalVirtualNetwork)
		if !ok {
			return nil, true, fmt.Errorf("manual backend server %q has no IP on network %q", serverName, strings.TrimSpace(lb.Spec.InternalVirtualNetwork))
		}
		if _, exists := seen[ip]; exists {
			continue
		}
		seen[ip] = struct{}{}
		resolved = append(resolved, ip)
	}

	if len(resolved) == 0 {
		return nil, false, fmt.Errorf("spec.internalServers is required when backendMode=manual")
	}
	sort.Strings(resolved)
	return resolved, false, nil
}

func (c *controller) resolveLoadBalancerVIP(lb api.LoadBalancer, vnet api.VirtualNetwork) (string, error) {
	if lb.Spec.VirtualIpAddress != nil {
		explicitVIP := strings.TrimSpace(*lb.Spec.VirtualIpAddress)
		if explicitVIP != "" {
			return explicitVIP, nil
		}
	}

	if lb.Metadata.Labels != nil {
		if vip := db.GetLoadBalancerResolvedVIP(*lb.Metadata.Labels); vip != "" {
			return vip, nil
		}
	}

	ipnetID := ""
	if vnet.Spec.IpNetworkId != nil {
		ipnetID = strings.TrimSpace(*vnet.Spec.IpNetworkId)
	}
	if ipnetID == "" {
		return "", fmt.Errorf("virtual network %q has no ipNetworkId for VIP auto allocation", strings.TrimSpace(vnet.Metadata.Name))
	}
	vnetID := strings.TrimSpace(api.VirtualNetworkID(vnet))
	if vnetID == "" {
		return "", fmt.Errorf("virtual network id is empty for %q", strings.TrimSpace(vnet.Metadata.Name))
	}
	lbID := strings.TrimSpace(api.LoadBalancerID(lb))
	if lbID == "" {
		return "", fmt.Errorf("load balancer id is empty")
	}

	allocated, _, err := c.db.AllocateIP(vnetID, ipnetID, "loadbalancer-"+lbID)
	if err != nil {
		return "", err
	}
	if err := c.updateLoadBalancerLabels(lbID, func(labels map[string]interface{}) {
		db.SetLoadBalancerResolvedVIP(labels, allocated)
	}); err != nil {
		return "", err
	}
	return allocated, nil
}

func (c *controller) releaseLoadBalancerAllocatedVIP(lb api.LoadBalancer) error {
	if lb.Metadata.Labels == nil {
		return nil
	}
	vip := db.GetLoadBalancerResolvedVIP(*lb.Metadata.Labels)
	if strings.TrimSpace(vip) == "" {
		return nil
	}
	vnet, err := c.db.GetVirtualNetworkByName(strings.TrimSpace(lb.Spec.InternalVirtualNetwork))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	vnetID := strings.TrimSpace(api.VirtualNetworkID(vnet))
	if vnetID == "" || vnet.Spec.IpNetworkId == nil || strings.TrimSpace(*vnet.Spec.IpNetworkId) == "" {
		return nil
	}
	if err := c.db.ReleaseIP(vnetID, strings.TrimSpace(*vnet.Spec.IpNetworkId), strings.TrimSpace(vip)); err != nil && !errors.Is(err, db.ErrNotFound) {
		return err
	}
	return nil
}

func (c *controller) updateLoadBalancerLabels(loadBalancerID string, mutate func(labels map[string]interface{})) error {
	lb, err := c.db.GetLoadBalancerById(loadBalancerID)
	if err != nil {
		return err
	}
	if lb.Metadata.Labels == nil {
		labels := map[string]interface{}{}
		lb.Metadata.Labels = &labels
	}
	labels := *lb.Metadata.Labels
	mutate(labels)
	lb.Metadata.Labels = &labels
	return c.db.UpdateLoadBalancerById(loadBalancerID, lb)
}

func (c *controller) assignLoadBalancerNode(loadBalancerID string) error {
	lb, err := c.db.GetLoadBalancerById(loadBalancerID)
	if err != nil {
		return err
	}
	if lb.Metadata.NodeName != nil && strings.TrimSpace(*lb.Metadata.NodeName) != "" {
		return nil
	}
	if strings.TrimSpace(c.marmot.NodeName) == "" {
		return fmt.Errorf("controller node name is empty")
	}
	lb.Metadata.NodeName = util.StringPtr(strings.TrimSpace(c.marmot.NodeName))
	return c.db.UpdateLoadBalancerById(loadBalancerID, lb)
}

func loadBalancerHasAssignedNode(lb api.LoadBalancer) bool {
	if lb.Metadata.NodeName == nil {
		return false
	}
	return strings.TrimSpace(*lb.Metadata.NodeName) != ""
}

func loadBalancerAppliedConfigHash(lb api.LoadBalancer) string {
	if lb.Metadata.Labels == nil {
		return ""
	}
	return db.GetLoadBalancerAppliedConfigHash(*lb.Metadata.Labels)
}

func loadBalancerLogicalSwitchNameFromLabels(lb api.LoadBalancer) string {
	if lb.Metadata.Labels == nil {
		return ""
	}
	value, ok := (*lb.Metadata.Labels)[loadBalancerLabelLogicalSwitch]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func resolveLoadBalancerBackendMode(mode *string) string {
	if mode == nil {
		return loadBalancerBackendModeAuto
	}
	trimmed := strings.ToLower(strings.TrimSpace(*mode))
	if trimmed == "" {
		return loadBalancerBackendModeAuto
	}
	return trimmed
}

func loadBalancerLogicalSwitchName(vnet api.VirtualNetwork) string {
	id := strings.TrimSpace(api.VirtualNetworkID(vnet))
	if id == "" {
		id = strings.TrimSpace(vnet.Metadata.Name)
	}
	if id == "" {
		return ""
	}
	return "marmot-net-" + loadBalancerLogicalSwitchSanitizer.ReplaceAllString(id, "-")
}

func desiredLoadBalancerConfigHash(lb api.LoadBalancer, mode string, vip string, ports []string, backends []string) string {
	sortedPorts := append([]string(nil), ports...)
	sortedBackends := append([]string(nil), backends...)
	sort.Strings(sortedPorts)
	sort.Strings(sortedBackends)

	payload := strings.Join([]string{
		strings.TrimSpace(api.LoadBalancerID(lb)),
		strings.TrimSpace(mode),
		strings.TrimSpace(lb.Spec.InternalVirtualNetwork),
		strings.TrimSpace(vip),
		strings.Join(sortedPorts, ","),
		strings.Join(sortedBackends, ","),
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum)
}

func withPort(ips []string, port string) []string {
	result := make([]string, 0, len(ips))
	for _, ip := range ips {
		result = append(result, fmt.Sprintf("%s:%s", strings.TrimSpace(ip), strings.TrimSpace(port)))
	}
	return result
}
