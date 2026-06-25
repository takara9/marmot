package controller

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

const (
	defaultApplicationLoadBalancerControllerIntervalSeconds         = 15
	defaultApplicationLoadBalancerAgentStateReadMaxFailures         = 3
	defaultApplicationLoadBalancerAgentStateRecoverySuccessRequired = 2
	applicationLoadBalancerApplyResultFreshnessThreshold            = 3 * time.Second
)

var (
	applicationLoadBalancerControllerInterval                = time.Duration(defaultApplicationLoadBalancerControllerIntervalSeconds) * time.Second
	applicationLoadBalancerAgentStateReadMaxFailures         = defaultApplicationLoadBalancerAgentStateReadMaxFailures
	applicationLoadBalancerAgentStateRecoverySuccessRequired = defaultApplicationLoadBalancerAgentStateRecoverySuccessRequired
)

// StartApplicationLoadBalancerController starts controller loop for load balancer resources.
func StartApplicationLoadBalancerController(node string, etcdUrl string) (*controller, error) {
	var c controller
	var err error
	applicationLoadBalancerControllerSettingsFromEnv()

	c.deletionDelay = 15 * time.Second
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	ticker := time.NewTicker(applicationLoadBalancerControllerInterval)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.applicationLoadBalancerControllerLoop()
			case <-c.stopChan:
				slog.Debug("ロードバランサーコントローラー停止")
				return
			}
		}
	}()

	return &c, nil
}

func (c *controller) applicationLoadBalancerControllerLoop() {
	slog.Debug("ロードバランサーコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))

	items, err := c.db.GetLoadBalancers()
	if err != nil {
		slog.Error("GetLoadBalancers() failed", "err", err)
		return
	}

	for _, item := range items {
		id := api.LoadBalancerID(item)
		if ok, assignedNode, reason := evaluateNodeAssignment(&item.Metadata, c.marmot.NodeName); !ok {
			slog.Debug("別ノード割当のロードバランサーをスキップ", "id", id, "name", item.Metadata.Name, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			continue
		}

		if item.Status == nil || item.Status.StatusCode != db.LOAD_BALANCER_DELETING {
			missingServer, err := c.isApplicationLoadBalancerManagedServerMissing(item)
			if err != nil {
				slog.Warn("failed to validate load balancer managed server", "id", id, "err", err)
				continue
			}
			if missingServer {
				c.deleteApplicationLoadBalancerForMissingServer(item)
				continue
			}
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
			c.reconcileApplicationLoadBalancerPending(item)
		case db.LOAD_BALANCER_PROVISIONING:
			c.reconcileApplicationLoadBalancerProvisioning(item)
		case db.LOAD_BALANCER_CONFIGURING:
			c.reconcileApplicationLoadBalancerConfiguring(item)
		case db.LOAD_BALANCER_ACTIVE, db.LOAD_BALANCER_DEGRADED:
			c.reconcileApplicationLoadBalancerActive(item)
		case db.LOAD_BALANCER_DELETING:
			c.reconcileApplicationLoadBalancerDeleting(item)
		case db.LOAD_BALANCER_FAILED:
			slog.Debug("ロードバランサー状態を監視", "id", id, "statusCode", item.Status.StatusCode)
		default:
			slog.Warn("不明なロードバランサー状態", "id", id, "statusCode", item.Status.StatusCode)
		}
	}
}

func (c *controller) isApplicationLoadBalancerManagedServerMissing(loadBalancer api.ApplicationLoadBalancer) (bool, error) {
	serverID := strings.TrimSpace(applicationLoadBalancerManagedServerID(loadBalancer))
	if serverID == "" {
		return false, nil
	}

	if _, err := c.db.GetServerById(serverID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func (c *controller) deleteApplicationLoadBalancerForMissingServer(loadBalancer api.ApplicationLoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	c.cleanupApplicationLoadBalancerDesiredConfig(loadBalancerID)
	if err := c.db.DeleteLoadBalancerById(loadBalancerID); err != nil {
		slog.Warn("DeleteLoadBalancerById() failed while auto-deleting load balancer", "id", loadBalancerID, "err", err)
		return
	}
	slog.Debug("load balancer deleted because managed server no longer exists", "id", loadBalancerID)
}

func (c *controller) reconcileApplicationLoadBalancerPending(loadBalancer api.ApplicationLoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)

	if err := validateGatewayInternalNetwork(c.db, loadBalancer.Spec.InternalVirtualNetwork); err != nil {
		if isRetryableApplicationLoadBalancerPendingError(err) {
			slog.Warn("validateGatewayInternalNetwork() failed with retryable error; keep load balancer pending", "id", loadBalancerID, "err", err)
			retryMsg := fmt.Sprintf("ロードバランサーのプロビジョニング待機中（依存関係の準備待ち）: %v", err)
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_PENDING, retryMsg)
			return
		}
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return
	}

	serverID, err := c.ensureApplicationLoadBalancerServerEntry(loadBalancer)
	if err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return
	}

	if err := c.ensureApplicationLoadBalancerManagedServerLabel(loadBalancerID, serverID); err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return
	}

	if err := c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_PROVISIONING, "load balancer VM provisioning started"); err != nil {
		slog.Warn("UpdateLoadBalancerStatusWithMessage() failed", "id", loadBalancerID, "err", err)
	}
}

func (c *controller) reconcileApplicationLoadBalancerProvisioning(loadBalancer api.ApplicationLoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	serverID := applicationLoadBalancerManagedServerID(loadBalancer)
	if strings.TrimSpace(serverID) == "" {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_PENDING, "load balancer server reference is missing")
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_PENDING, "load balancer server not found, recreating")
			return
		}
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return
	}

	if server.Status == nil {
		return
	}

	switch server.Status.StatusCode {
	case db.SERVER_RUNNING:
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, "load balancer VM running, applying ansible")
	case db.SERVER_ERROR:
		msg := "load balancer server entered error state"
		if server.Status.Message != nil && strings.TrimSpace(*server.Status.Message) != "" {
			msg = strings.TrimSpace(*server.Status.Message)
		}
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, msg)
	}
}

func (c *controller) reconcileApplicationLoadBalancerConfiguring(loadBalancer api.ApplicationLoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	serverID := applicationLoadBalancerManagedServerID(loadBalancer)
	if strings.TrimSpace(serverID) == "" {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_PENDING, "load balancer server reference is missing")
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_PENDING, "load balancer server not found, recreating")
			return
		}
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return
	}
	if server.Status == nil || server.Status.StatusCode != db.SERVER_RUNNING {
		return
	}

	targetIP, err := c.resolveApplicationLoadBalancerTargetAddress(loadBalancer)
	if err != nil {
		c.handleApplicationLoadBalancerConfigFailure(loadBalancerID, err)
		return
	}
	listenerBackends, err := c.resolveApplicationLoadBalancerListenerBackends(loadBalancer)
	if err != nil {
		c.handleApplicationLoadBalancerConfigFailure(loadBalancerID, err)
		return
	}
	if msg := applicationLoadBalancerBackendAvailabilityMessage(loadBalancer, listenerBackends); msg != "" {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, msg)
		return
	}

	playbookPath := filepath.Join(applicationLoadBalancerPlaybookDir, fmt.Sprintf("load-balancer-%s.yaml", loadBalancerID))
	desiredConfigPath := applicationLoadBalancerDesiredConfigPath(loadBalancerID)
	configHash, err := writeApplicationLoadBalancerDesiredConfig(desiredConfigPath, loadBalancer, listenerBackends)
	if err != nil {
		c.handleApplicationLoadBalancerConfigFailure(loadBalancerID, err)
		return
	}
	if applicationLoadBalancerStagedConfigHash(loadBalancer) != configHash {
		if err := renderApplicationLoadBalancerPlaybook(playbookPath, targetIP, desiredConfigPath, desiredConfigPath); err != nil {
			c.handleApplicationLoadBalancerConfigFailure(loadBalancerID, err)
			return
		}
		if err := runApplicationLoadBalancerPlaybook(playbookPath, targetIP, applicationLoadBalancerPrivateKeyPath); err != nil {
			c.handleApplicationLoadBalancerConfigFailure(loadBalancerID, err)
			return
		}
		if err := c.updateApplicationLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
			db.SetLoadBalancerAnsibleRetries(labels, 0)
			db.SetLoadBalancerStagedConfigHash(labels, configHash)
			db.SetLoadBalancerStagedConfigAt(labels, time.Now().UTC())
			db.SetLoadBalancerAgentStateReadFailures(labels, 0)
			db.SetLoadBalancerAgentStateReadSuccesses(labels, 0)
		}); err != nil {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
			return
		}
		refreshed, err := c.db.GetLoadBalancerById(loadBalancerID)
		if err == nil {
			loadBalancer = refreshed
		}
	}
	c.observeApplicationLoadBalancerAgentState(loadBalancer, targetIP, configHash, applicationLoadBalancerStagedConfigAt(loadBalancer), desiredConfigPath)
}

func (c *controller) reconcileApplicationLoadBalancerActive(loadBalancer api.ApplicationLoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	serverID := applicationLoadBalancerManagedServerID(loadBalancer)
	if strings.TrimSpace(serverID) == "" {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_PENDING, "load balancer server reference is missing")
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_PENDING, "load balancer server disappeared")
			return
		}
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return
	}

	if server.Status != nil && server.Status.StatusCode == db.SERVER_ERROR {
		msg := "load balancer server entered error state"
		if server.Status.Message != nil && strings.TrimSpace(*server.Status.Message) != "" {
			msg = strings.TrimSpace(*server.Status.Message)
		}
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, msg)
		return
	}

	listenerBackends, err := c.resolveApplicationLoadBalancerListenerBackends(loadBalancer)
	if err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, err.Error())
		return
	}
	if msg := applicationLoadBalancerBackendAvailabilityMessage(loadBalancer, listenerBackends); msg != "" {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, msg)
		return
	}

	desiredHash, err := desiredApplicationLoadBalancerConfigHash(loadBalancer, listenerBackends)
	if err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return
	}

	if applicationLoadBalancerAppliedConfigHash(loadBalancer) != desiredHash {
		if err := c.updateApplicationLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
			db.SetLoadBalancerAnsibleRetries(labels, 0)
			db.SetLoadBalancerStagedConfigHash(labels, "")
			db.SetLoadBalancerStagedConfigAt(labels, time.Time{})
			db.SetLoadBalancerAgentStateReadFailures(labels, 0)
			db.SetLoadBalancerAgentStateReadSuccesses(labels, 0)
		}); err != nil {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
			return
		}
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, "load balancer configuration drift detected")
		return
	}

	targetIP, err := c.resolveApplicationLoadBalancerTargetAddress(loadBalancer)
	if err == nil {
		if observed := c.observeApplicationLoadBalancerAgentState(loadBalancer, targetIP, applicationLoadBalancerAppliedConfigHash(loadBalancer), applicationLoadBalancerStagedConfigAt(loadBalancer), ""); observed {
			return
		}
	}

	if loadBalancer.Status != nil && loadBalancer.Status.StatusCode == db.LOAD_BALANCER_DEGRADED {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_ACTIVE, "")
	}
}

func (c *controller) resolveApplicationLoadBalancerListenerBackends(loadBalancer api.ApplicationLoadBalancer) (map[string][]applicationLoadBalancerBackendServer, error) {
	servers, err := c.db.GetServers()
	if err != nil {
		return nil, err
	}

	internalNetwork := strings.TrimSpace(loadBalancer.Spec.InternalVirtualNetwork)
	result := make(map[string][]applicationLoadBalancerBackendServer, len(loadBalancer.Spec.Listeners))
	for _, listener := range loadBalancer.Spec.Listeners {
		listenerName := strings.TrimSpace(listener.Name)
		if listenerName == "" {
			continue
		}
		items := make([]applicationLoadBalancerBackendServer, 0)
		for _, server := range servers {
			if server.Status == nil || server.Status.StatusCode != db.SERVER_RUNNING {
				continue
			}
			if !serverMatchesApplicationLoadBalancerSelector(server, listener.BackendSelector.MatchLabels) {
				continue
			}
			ip := serverAddressInNetwork(server, internalNetwork)
			if ip == "" {
				continue
			}
			name := strings.TrimSpace(server.Metadata.Name)
			if name == "" {
				name = strings.TrimSpace(api.ServerID(server))
			}
			if name == "" {
				continue
			}
			items = append(items, applicationLoadBalancerBackendServer{Name: name, IP: ip})
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Name == items[j].Name {
				return items[i].IP < items[j].IP
			}
			return items[i].Name < items[j].Name
		})
		result[listenerName] = items
	}
	return result, nil
}

func serverMatchesApplicationLoadBalancerSelector(server api.Server, matchLabels map[string]string) bool {
	if len(matchLabels) == 0 {
		return false
	}
	if server.Metadata.Labels == nil {
		return false
	}
	labels := *server.Metadata.Labels
	for key, expected := range matchLabels {
		value, ok := labels[strings.TrimSpace(key)]
		if !ok {
			return false
		}
		if strings.TrimSpace(fmt.Sprint(value)) != strings.TrimSpace(expected) {
			return false
		}
	}
	return true
}

func serverAddressInNetwork(server api.Server, networkName string) string {
	if server.Spec.NetworkInterface == nil {
		return ""
	}
	targetNetwork := strings.TrimSpace(networkName)
	for _, nic := range *server.Spec.NetworkInterface {
		if strings.TrimSpace(nic.Networkname) != targetNetwork || nic.Address == nil {
			continue
		}
		ip := strings.TrimSpace(*nic.Address)
		if ip != "" {
			return ip
		}
	}
	return ""
}

func applicationLoadBalancerBackendAvailabilityMessage(loadBalancer api.ApplicationLoadBalancer, listenerBackends map[string][]applicationLoadBalancerBackendServer) string {
	missing := make([]string, 0)
	for _, listener := range loadBalancer.Spec.Listeners {
		name := strings.TrimSpace(listener.Name)
		if name == "" {
			continue
		}
		if len(listenerBackends[name]) == 0 {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	sort.Strings(missing)
	return fmt.Sprintf("no backend matched for listener(s): %s", strings.Join(missing, ","))
}

func (c *controller) reconcileApplicationLoadBalancerDeleting(loadBalancer api.ApplicationLoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	c.cleanupApplicationLoadBalancerDesiredConfig(loadBalancerID)
	serverID := applicationLoadBalancerManagedServerID(loadBalancer)
	if strings.TrimSpace(serverID) == "" {
		if err := c.db.DeleteLoadBalancerById(loadBalancerID); err != nil {
			slog.Warn("DeleteLoadBalancerById() failed", "id", loadBalancerID, "err", err)
		}
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			if err := c.db.DeleteLoadBalancerById(loadBalancerID); err != nil {
				slog.Warn("DeleteLoadBalancerById() failed", "id", loadBalancerID, "err", err)
			}
			return
		}
		slog.Warn("GetServerById() failed while deleting load balancer", "id", loadBalancerID, "serverId", serverID, "err", err)
		return
	}

	if server.Status == nil || server.Status.StatusCode != db.SERVER_DELETING {
		if err := c.db.SetDeleteTimestamp(serverID); err != nil {
			slog.Warn("SetDeleteTimestamp() failed", "id", loadBalancerID, "serverId", serverID, "err", err)
		}
	}
}

func (c *controller) cleanupApplicationLoadBalancerDesiredConfig(loadBalancerID string) {
	path := applicationLoadBalancerDesiredConfigPath(loadBalancerID)
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("failed to remove load balancer desired config file", "id", loadBalancerID, "path", path, "err", err)
	}
}

func (c *controller) ensureApplicationLoadBalancerManagedServerLabel(loadBalancerID string, serverID string) error {
	loadBalancer, err := c.db.GetLoadBalancerById(loadBalancerID)
	if err != nil {
		return err
	}
	if loadBalancer.Metadata.Labels == nil {
		labels := map[string]interface{}{}
		loadBalancer.Metadata.Labels = &labels
	}
	labels := *loadBalancer.Metadata.Labels
	if db.GetLoadBalancerManagedServerID(labels) == strings.TrimSpace(serverID) {
		return nil
	}
	db.SetLoadBalancerManagedServerID(labels, serverID)
	loadBalancer.Metadata.Labels = &labels
	return c.db.UpdateLoadBalancerById(loadBalancerID, loadBalancer)
}

func (c *controller) updateApplicationLoadBalancerLabels(loadBalancerID string, mutate func(labels map[string]interface{})) error {
	loadBalancer, err := c.db.GetLoadBalancerById(loadBalancerID)
	if err != nil {
		return err
	}
	if loadBalancer.Metadata.Labels == nil {
		labels := map[string]interface{}{}
		loadBalancer.Metadata.Labels = &labels
	}
	labels := *loadBalancer.Metadata.Labels
	mutate(labels)
	loadBalancer.Metadata.Labels = &labels
	return c.db.UpdateLoadBalancerById(loadBalancerID, loadBalancer)
}

func (c *controller) ensureApplicationLoadBalancerServerEntry(loadBalancer api.ApplicationLoadBalancer) (string, error) {
	if serverID := applicationLoadBalancerManagedServerID(loadBalancer); strings.TrimSpace(serverID) != "" {
		if _, err := c.db.GetServerById(serverID); err == nil {
			return serverID, nil
		}
	}

	serverName := applicationLoadBalancerServerName(loadBalancer)
	if existing, err := c.findServerByName(serverName); err == nil {
		return api.ServerID(existing), nil
	}

	serverSpec, err := c.buildApplicationLoadBalancerServerSpec(loadBalancer, serverName)
	if err != nil {
		return "", err
	}

	created, err := c.db.MakeServerEntry(serverSpec)
	if err != nil {
		return "", err
	}

	return api.ServerID(created), nil
}

func (c *controller) buildApplicationLoadBalancerServerSpec(loadBalancer api.ApplicationLoadBalancer, serverName string) (api.Server, error) {
	publicIP, cidrMaskLen, err := normalizePublicBindAddress(loadBalancer.Spec.BindPublicIpAddress)
	if err != nil {
		return api.Server{}, fmt.Errorf("invalid bindPublicIpAddress: %w", err)
	}
	if publicIP == "" {
		return api.Server{}, fmt.Errorf("load balancer bindPublicIpAddress is empty")
	}
	internalNetwork := strings.TrimSpace(loadBalancer.Spec.InternalVirtualNetwork)
	if internalNetwork == "" {
		return api.Server{}, fmt.Errorf("load balancer internalVirtualNetwork is empty")
	}
	publicNetwork := applicationLoadBalancerPublicNetworkName(loadBalancer.Spec)

	nics := make([]api.NetworkInterface, 0, 2)
	publicNIC := api.NetworkInterface{Networkname: publicNetwork, Address: util.StringPtr(publicIP)}
	if cidrMaskLen > 0 {
		publicNIC.Netmasklen = util.IntPtrInt(cidrMaskLen)
	} else if maskLen, err := c.lookupNetworkMaskLen(publicNetwork); err == nil && maskLen > 0 {
		publicNIC.Netmasklen = util.IntPtrInt(maskLen)
	}
	if loadBalancer.Spec.Routes != nil && len(*loadBalancer.Spec.Routes) > 0 {
		publicNIC.Routes = loadBalancer.Spec.Routes
	} else if gw, err := c.lookupNetworkGateway(publicNetwork); err == nil && strings.TrimSpace(gw) != "" {
		defaultTo := "default"
		publicNIC.Routes = &[]api.Route{{To: &defaultTo, Via: util.StringPtr(gw)}}
	}
	nics = append(nics, publicNIC)

	internalNIC := api.NetworkInterface{Networkname: internalNetwork}
	if ip, err := c.deriveGatewayInternalInterfaceAddress(internalNetwork); err == nil && strings.TrimSpace(ip) != "" {
		internalNIC.Address = util.StringPtr(ip)
	}
	nics = append(nics, internalNIC)

	labels := map[string]interface{}{
		db.LoadBalancerServerLabelID:   api.LoadBalancerID(loadBalancer),
		db.LoadBalancerServerLabelRole: db.LoadBalancerServerLabelRoleValue,
		db.LoadBalancerLabelManagedBy:  db.LoadBalancerLabelManagedByValue,
	}

	meta := api.Metadata{Name: serverName, Labels: &labels}
	if loadBalancer.Metadata.NodeName != nil {
		node := strings.TrimSpace(*loadBalancer.Metadata.NodeName)
		if node != "" {
			meta.NodeName = util.StringPtr(node)
		}
	}

	spec := api.ServerSpec{
		Cpu:              util.IntPtrInt(1),
		Memory:           util.IntPtrInt(2048),
		OsVariant:        util.StringPtr("ubuntu24.04"),
		NetworkInterface: &nics,
	}

	if key := readGatewayPublicKeyIfExists(); key != "" {
		spec.Auth = &api.Auth{PublicKey: util.StringPtr(key), User: util.StringPtr("root")}
	}

	return api.Server{ApiVersion: "v1", Kind: "Server", Metadata: meta, Spec: spec}, nil
}

func applicationLoadBalancerManagedServerID(loadBalancer api.ApplicationLoadBalancer) string {
	if loadBalancer.Metadata.Labels == nil {
		return ""
	}
	return db.GetLoadBalancerManagedServerID(*loadBalancer.Metadata.Labels)
}

func applicationLoadBalancerAppliedConfigHash(loadBalancer api.ApplicationLoadBalancer) string {
	if loadBalancer.Metadata.Labels == nil {
		return ""
	}
	return db.GetLoadBalancerAppliedConfigHash(*loadBalancer.Metadata.Labels)
}

func applicationLoadBalancerStagedConfigHash(loadBalancer api.ApplicationLoadBalancer) string {
	if loadBalancer.Metadata.Labels == nil {
		return ""
	}
	return db.GetLoadBalancerStagedConfigHash(*loadBalancer.Metadata.Labels)
}

func applicationLoadBalancerStagedConfigAt(loadBalancer api.ApplicationLoadBalancer) time.Time {
	if loadBalancer.Metadata.Labels == nil {
		return time.Time{}
	}
	return db.GetLoadBalancerStagedConfigAt(*loadBalancer.Metadata.Labels)
}

func (c *controller) observeApplicationLoadBalancerAgentState(loadBalancer api.ApplicationLoadBalancer, targetIP, desiredHash string, stagedAt time.Time, desiredConfigPath string) bool {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	if strings.TrimSpace(desiredConfigPath) != "" {
		localHash, err := fileSHA256Hex(desiredConfigPath)
		if err != nil {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, fmt.Sprintf("failed to read local desired config hash: %v", err))
			return true
		}
		remoteHash, err := readApplicationLoadBalancerDesiredConfigHash(targetIP, applicationLoadBalancerPrivateKeyPath, desiredConfigPath)
		if err != nil {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, fmt.Sprintf("waiting for desired config hash from load balancer VM: %v", err))
			return true
		}
		if strings.TrimSpace(localHash) != strings.TrimSpace(remoteHash) {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, "waiting for desired config sync between marmotd and load balancer VM")
			return true
		}
		desiredHash = localHash
	}

	state, err := readApplicationLoadBalancerAgentState(targetIP, applicationLoadBalancerPrivateKeyPath)
	if err != nil {
		failures, labelErr := c.recordApplicationLoadBalancerAgentStateReadFailure(loadBalancerID)
		if labelErr != nil {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, labelErr.Error())
			return true
		}
		message := fmt.Sprintf("load balancer agent state unavailable (%d/%d): %v", failures, applicationLoadBalancerAgentStateReadMaxFailures, err)
		if failures >= applicationLoadBalancerAgentStateReadMaxFailures {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, message)
			return true
		}
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, message)
		return true
	}
	successes, err := c.recordApplicationLoadBalancerAgentStateReadSuccess(loadBalancerID)
	if err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return true
	}
	if strings.TrimSpace(state.LastError) != "" {
		_ = c.resetApplicationLoadBalancerAgentStateReadSuccesses(loadBalancerID)
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, state.LastError)
		return true
	}
	if strings.TrimSpace(state.LastAppliedHash) != strings.TrimSpace(desiredHash) {
		_ = c.resetApplicationLoadBalancerAgentStateReadSuccesses(loadBalancerID)
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, "waiting for load balancer agent to apply desired config")
		return true
	}
	if applicationLoadBalancerAgentApplyResultIsStale(state.LastAppliedAt, stagedAt) {
		_ = c.resetApplicationLoadBalancerAgentStateReadSuccesses(loadBalancerID)
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, "waiting for newer load balancer agent apply result")
		return true
	}
	if err := c.updateApplicationLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		db.SetLoadBalancerAppliedConfigHash(labels, desiredHash)
		db.SetLoadBalancerStagedConfigHash(labels, desiredHash)
		db.SetLoadBalancerStagedConfigAt(labels, stagedAt)
	}); err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return true
	}
	if loadBalancer.Status != nil && loadBalancer.Status.StatusCode == db.LOAD_BALANCER_DEGRADED && successes < applicationLoadBalancerAgentStateRecoverySuccessRequired {
		msg := fmt.Sprintf("waiting for consecutive successful load balancer agent checks (%d/%d)", successes, applicationLoadBalancerAgentStateRecoverySuccessRequired)
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, msg)
		return true
	}
	_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_ACTIVE, "")
	return true
}

func applicationLoadBalancerAgentApplyResultIsStale(lastAppliedAt, stagedAt time.Time) bool {
	if stagedAt.IsZero() {
		return false
	}
	if lastAppliedAt.IsZero() {
		return true
	}
	return stagedAt.UTC().Sub(lastAppliedAt.UTC()) >= applicationLoadBalancerApplyResultFreshnessThreshold
}

func (c *controller) resolveApplicationLoadBalancerTargetAddress(loadBalancer api.ApplicationLoadBalancer) (string, error) {
	serverID := strings.TrimSpace(applicationLoadBalancerManagedServerID(loadBalancer))
	if serverID == "" {
		return "", fmt.Errorf("load balancer server reference is missing")
	}
	server, err := c.db.GetServerById(serverID)
	if err != nil {
		return "", err
	}
	if server.Spec.NetworkInterface == nil {
		return "", fmt.Errorf("load balancer server has no network interface")
	}
	publicIP, _, err := normalizePublicBindAddress(loadBalancer.Spec.BindPublicIpAddress)
	if err != nil {
		return "", fmt.Errorf("invalid bindPublicIpAddress: %w", err)
	}
	publicNetwork := applicationLoadBalancerPublicNetworkName(loadBalancer.Spec)
	for _, nic := range *server.Spec.NetworkInterface {
		if nic.Address != nil {
			if ip := strings.TrimSpace(*nic.Address); publicIP != "" && ip == publicIP {
				return ip, nil
			}
		}
		if strings.TrimSpace(nic.Networkname) == publicNetwork && nic.Address != nil {
			if ip := strings.TrimSpace(*nic.Address); ip != "" {
				return ip, nil
			}
		}
	}
	if publicIP != "" {
		return publicIP, nil
	}
	return "", fmt.Errorf("load balancer public target address is missing")
}

func applicationLoadBalancerPublicNetworkName(spec api.ApplicationLoadBalancerSpec) string {
	if spec.BindPublicNetworkName != nil {
		if network := strings.TrimSpace(*spec.BindPublicNetworkName); network != "" {
			return network
		}
	}
	return "host-bridge"
}

func normalizePublicBindAddress(raw string) (string, int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", 0, nil
	}
	if strings.Contains(trimmed, "/") {
		ip, ipNet, err := net.ParseCIDR(trimmed)
		if err != nil {
			return "", 0, err
		}
		bits, _ := ipNet.Mask.Size()
		return strings.TrimSpace(ip.String()), bits, nil
	}
	if parsed := net.ParseIP(trimmed); parsed != nil {
		return strings.TrimSpace(parsed.String()), 0, nil
	}
	return "", 0, fmt.Errorf("invalid ip address %q", trimmed)
}

func (c *controller) handleApplicationLoadBalancerConfigFailure(loadBalancerID string, err error) {
	if err == nil {
		return
	}
	retries, labelErr := c.incrementApplicationLoadBalancerConfigRetries(loadBalancerID)
	if labelErr != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, labelErr.Error())
		return
	}
	message := fmt.Sprintf("load balancer ansible apply failed (%d/%d): %v", retries, applicationLoadBalancerAnsibleMaxRetryCount, err)
	if retries >= applicationLoadBalancerAnsibleMaxRetryCount {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, message)
		return
	}
	_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, message)
}

func (c *controller) incrementApplicationLoadBalancerConfigRetries(loadBalancerID string) (int, error) {
	next := 0
	err := c.updateApplicationLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		next = db.GetLoadBalancerAnsibleRetries(labels) + 1
		db.SetLoadBalancerAnsibleRetries(labels, next)
	})
	return next, err
}

func (c *controller) recordApplicationLoadBalancerAgentStateReadFailure(loadBalancerID string) (int, error) {
	next := 0
	err := c.updateApplicationLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		next = db.GetLoadBalancerAgentStateReadFailures(labels) + 1
		db.SetLoadBalancerAgentStateReadFailures(labels, next)
		db.SetLoadBalancerAgentStateReadSuccesses(labels, 0)
	})
	return next, err
}

func (c *controller) recordApplicationLoadBalancerAgentStateReadSuccess(loadBalancerID string) (int, error) {
	next := 0
	err := c.updateApplicationLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		db.SetLoadBalancerAgentStateReadFailures(labels, 0)
		next = db.GetLoadBalancerAgentStateReadSuccesses(labels) + 1
		db.SetLoadBalancerAgentStateReadSuccesses(labels, next)
	})
	return next, err
}

func (c *controller) resetApplicationLoadBalancerAgentStateReadSuccesses(loadBalancerID string) error {
	return c.updateApplicationLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		db.SetLoadBalancerAgentStateReadSuccesses(labels, 0)
	})
}

func applicationLoadBalancerServerName(loadBalancer api.ApplicationLoadBalancer) string {
	id := strings.TrimSpace(api.LoadBalancerID(loadBalancer))
	if id == "" {
		id = strings.TrimSpace(loadBalancer.Metadata.Name)
	}
	return fmt.Sprintf("lb-%s", id)
}

func applicationLoadBalancerControllerSettingsFromEnv() {
	intervalSeconds := readPositiveIntEnv("MARMOT_LB_CONTROLLER_INTERVAL_SECONDS", defaultApplicationLoadBalancerControllerIntervalSeconds)
	applicationLoadBalancerControllerInterval = time.Duration(intervalSeconds) * time.Second
	applicationLoadBalancerAgentStateReadMaxFailures = readPositiveIntEnv("MARMOT_LB_AGENT_STATE_READ_MAX_FAILURES", defaultApplicationLoadBalancerAgentStateReadMaxFailures)
	applicationLoadBalancerAgentStateRecoverySuccessRequired = readPositiveIntEnv("MARMOT_LB_AGENT_RECOVERY_SUCCESS_REQUIRED", defaultApplicationLoadBalancerAgentStateRecoverySuccessRequired)
}

func readPositiveIntEnv(name string, fallback int) int {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

// isRetryableApplicationLoadBalancerPendingError は、PENDING フェーズの ALB が
// 再試行すべき一時的な依存エラーかどうかを判定する。
// 仮想ネットワークが未作成（ACTIVEでない）の場合は再試行対象とする。
func isRetryableApplicationLoadBalancerPendingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	// validateGatewayInternalNetwork が返す "internalVirtualNetwork \"X\" is not found"
	return strings.Contains(msg, "internalvirtualnetwork") && strings.Contains(msg, "is not found")
}
