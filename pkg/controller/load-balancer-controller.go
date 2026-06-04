package controller

import (
	"errors"
	"fmt"
	"log/slog"
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
	defaultLoadBalancerControllerIntervalSeconds          = 15
	defaultLoadBalancerAgentStateReadMaxFailures          = 3
	defaultLoadBalancerAgentStateRecoverySuccessRequired  = 2
)

var (
	loadBalancerControllerInterval = time.Duration(defaultLoadBalancerControllerIntervalSeconds) * time.Second
	loadBalancerAgentStateReadMaxFailures = defaultLoadBalancerAgentStateReadMaxFailures
	loadBalancerAgentStateRecoverySuccessRequired = defaultLoadBalancerAgentStateRecoverySuccessRequired
)

// StartLoadBalancerController starts controller loop for load balancer resources.
func StartLoadBalancerController(node string, etcdUrl string) (*controller, error) {
	var c controller
	var err error
	loadBalancerControllerSettingsFromEnv()

	c.deletionDelay = 15 * time.Second
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	ticker := time.NewTicker(loadBalancerControllerInterval)
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

	for _, item := range items {
		id := api.LoadBalancerID(item)
		if ok, assignedNode, reason := evaluateNodeAssignment(&item.Metadata, c.marmot.NodeName); !ok {
			slog.Debug("別ノード割当のロードバランサーをスキップ", "id", id, "name", item.Metadata.Name, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			continue
		}

		if item.Status == nil || item.Status.StatusCode != db.LOAD_BALANCER_DELETING {
			missingServer, err := c.isLoadBalancerManagedServerMissing(item)
			if err != nil {
				slog.Warn("failed to validate load balancer managed server", "id", id, "err", err)
				continue
			}
			if missingServer {
				c.deleteLoadBalancerForMissingServer(item)
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
			c.reconcileLoadBalancerPending(item)
		case db.LOAD_BALANCER_PROVISIONING:
			c.reconcileLoadBalancerProvisioning(item)
		case db.LOAD_BALANCER_CONFIGURING:
			c.reconcileLoadBalancerConfiguring(item)
		case db.LOAD_BALANCER_ACTIVE, db.LOAD_BALANCER_DEGRADED:
			c.reconcileLoadBalancerActive(item)
		case db.LOAD_BALANCER_DELETING:
			c.reconcileLoadBalancerDeleting(item)
		case db.LOAD_BALANCER_FAILED:
			slog.Debug("ロードバランサー状態を監視", "id", id, "statusCode", item.Status.StatusCode)
		default:
			slog.Warn("不明なロードバランサー状態", "id", id, "statusCode", item.Status.StatusCode)
		}
	}
}

func (c *controller) isLoadBalancerManagedServerMissing(loadBalancer api.LoadBalancer) (bool, error) {
	serverID := strings.TrimSpace(loadBalancerManagedServerID(loadBalancer))
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

func (c *controller) deleteLoadBalancerForMissingServer(loadBalancer api.LoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	if err := c.db.DeleteLoadBalancerById(loadBalancerID); err != nil {
		slog.Warn("DeleteLoadBalancerById() failed while auto-deleting load balancer", "id", loadBalancerID, "err", err)
		return
	}
	slog.Info("load balancer deleted because managed server no longer exists", "id", loadBalancerID)
}

func (c *controller) reconcileLoadBalancerPending(loadBalancer api.LoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)

	if err := validateGatewayInternalNetwork(c.db, loadBalancer.Spec.InternalVirtualNetwork); err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return
	}

	serverID, err := c.ensureLoadBalancerServerEntry(loadBalancer)
	if err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return
	}

	if err := c.ensureLoadBalancerManagedServerLabel(loadBalancerID, serverID); err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return
	}

	if err := c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_PROVISIONING, "load balancer VM provisioning started"); err != nil {
		slog.Warn("UpdateLoadBalancerStatusWithMessage() failed", "id", loadBalancerID, "err", err)
	}
}

func (c *controller) reconcileLoadBalancerProvisioning(loadBalancer api.LoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	serverID := loadBalancerManagedServerID(loadBalancer)
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

func (c *controller) reconcileLoadBalancerConfiguring(loadBalancer api.LoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	serverID := loadBalancerManagedServerID(loadBalancer)
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

	targetIP, err := c.resolveLoadBalancerTargetAddress(loadBalancer)
	if err != nil {
		c.handleLoadBalancerConfigFailure(loadBalancerID, err)
		return
	}
	listenerBackends, err := c.resolveLoadBalancerListenerBackends(loadBalancer)
	if err != nil {
		c.handleLoadBalancerConfigFailure(loadBalancerID, err)
		return
	}
	if msg := loadBalancerBackendAvailabilityMessage(loadBalancer, listenerBackends); msg != "" {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, msg)
		return
	}

	playbookPath := filepath.Join(loadBalancerPlaybookDir, fmt.Sprintf("load-balancer-%s.yaml", loadBalancerID))
	configHash := desiredLoadBalancerConfigHash(loadBalancer, listenerBackends)
	if loadBalancerStagedConfigHash(loadBalancer) != configHash {
		if err := renderLoadBalancerPlaybook(playbookPath, targetIP, loadBalancer, listenerBackends); err != nil {
			c.handleLoadBalancerConfigFailure(loadBalancerID, err)
			return
		}
		if err := runLoadBalancerPlaybook(playbookPath, targetIP, loadBalancerPrivateKeyPath); err != nil {
			c.handleLoadBalancerConfigFailure(loadBalancerID, err)
			return
		}
		if err := c.updateLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
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
	c.observeLoadBalancerAgentState(loadBalancer, targetIP, configHash, loadBalancerStagedConfigAt(loadBalancer))
}

func (c *controller) reconcileLoadBalancerActive(loadBalancer api.LoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	serverID := loadBalancerManagedServerID(loadBalancer)
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

	listenerBackends, err := c.resolveLoadBalancerListenerBackends(loadBalancer)
	if err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, err.Error())
		return
	}
	if msg := loadBalancerBackendAvailabilityMessage(loadBalancer, listenerBackends); msg != "" {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, msg)
		return
	}

	if loadBalancerAppliedConfigHash(loadBalancer) != desiredLoadBalancerConfigHash(loadBalancer, listenerBackends) {
		if err := c.updateLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
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

	targetIP, err := c.resolveLoadBalancerTargetAddress(loadBalancer)
	if err == nil {
		if observed := c.observeLoadBalancerAgentState(loadBalancer, targetIP, loadBalancerAppliedConfigHash(loadBalancer), loadBalancerStagedConfigAt(loadBalancer)); observed {
			return
		}
	}

	if loadBalancer.Status != nil && loadBalancer.Status.StatusCode == db.LOAD_BALANCER_DEGRADED {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_ACTIVE, "")
	}
}

func (c *controller) resolveLoadBalancerListenerBackends(loadBalancer api.LoadBalancer) (map[string][]loadBalancerBackendServer, error) {
	servers, err := c.db.GetServers()
	if err != nil {
		return nil, err
	}

	internalNetwork := strings.TrimSpace(loadBalancer.Spec.InternalVirtualNetwork)
	result := make(map[string][]loadBalancerBackendServer, len(loadBalancer.Spec.Listeners))
	for _, listener := range loadBalancer.Spec.Listeners {
		listenerName := strings.TrimSpace(listener.Name)
		if listenerName == "" {
			continue
		}
		items := make([]loadBalancerBackendServer, 0)
		for _, server := range servers {
			if server.Status == nil || server.Status.StatusCode != db.SERVER_RUNNING {
				continue
			}
			if !serverMatchesLoadBalancerSelector(server, listener.BackendSelector.MatchLabels) {
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
			items = append(items, loadBalancerBackendServer{Name: name, IP: ip})
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

func serverMatchesLoadBalancerSelector(server api.Server, matchLabels map[string]string) bool {
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

func loadBalancerBackendAvailabilityMessage(loadBalancer api.LoadBalancer, listenerBackends map[string][]loadBalancerBackendServer) string {
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

func (c *controller) reconcileLoadBalancerDeleting(loadBalancer api.LoadBalancer) {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	serverID := loadBalancerManagedServerID(loadBalancer)
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

func (c *controller) ensureLoadBalancerManagedServerLabel(loadBalancerID string, serverID string) error {
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

func (c *controller) updateLoadBalancerLabels(loadBalancerID string, mutate func(labels map[string]interface{})) error {
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

func (c *controller) ensureLoadBalancerServerEntry(loadBalancer api.LoadBalancer) (string, error) {
	if serverID := loadBalancerManagedServerID(loadBalancer); strings.TrimSpace(serverID) != "" {
		if _, err := c.db.GetServerById(serverID); err == nil {
			return serverID, nil
		}
	}

	serverName := loadBalancerServerName(loadBalancer)
	if existing, err := c.findServerByName(serverName); err == nil {
		return api.ServerID(existing), nil
	}

	serverSpec, err := c.buildLoadBalancerServerSpec(loadBalancer, serverName)
	if err != nil {
		return "", err
	}

	created, err := c.db.MakeServerEntry(serverSpec)
	if err != nil {
		return "", err
	}

	return api.ServerID(created), nil
}

func (c *controller) buildLoadBalancerServerSpec(loadBalancer api.LoadBalancer, serverName string) (api.Server, error) {
	publicIP := strings.TrimSpace(loadBalancer.Spec.BindPublicIpAddress)
	if publicIP == "" {
		return api.Server{}, fmt.Errorf("load balancer bindPublicIpAddress is empty")
	}
	internalNetwork := strings.TrimSpace(loadBalancer.Spec.InternalVirtualNetwork)
	if internalNetwork == "" {
		return api.Server{}, fmt.Errorf("load balancer internalVirtualNetwork is empty")
	}

	nics := make([]api.NetworkInterface, 0, 2)
	publicNIC := api.NetworkInterface{Networkname: "host-bridge", Address: util.StringPtr(publicIP)}
	if maskLen, err := c.lookupNetworkMaskLen("host-bridge"); err == nil && maskLen > 0 {
		publicNIC.Netmasklen = util.IntPtrInt(maskLen)
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

func loadBalancerManagedServerID(loadBalancer api.LoadBalancer) string {
	if loadBalancer.Metadata.Labels == nil {
		return ""
	}
	return db.GetLoadBalancerManagedServerID(*loadBalancer.Metadata.Labels)
}

func loadBalancerAppliedConfigHash(loadBalancer api.LoadBalancer) string {
	if loadBalancer.Metadata.Labels == nil {
		return ""
	}
	return db.GetLoadBalancerAppliedConfigHash(*loadBalancer.Metadata.Labels)
}

func loadBalancerStagedConfigHash(loadBalancer api.LoadBalancer) string {
	if loadBalancer.Metadata.Labels == nil {
		return ""
	}
	return db.GetLoadBalancerStagedConfigHash(*loadBalancer.Metadata.Labels)
}

func loadBalancerStagedConfigAt(loadBalancer api.LoadBalancer) time.Time {
	if loadBalancer.Metadata.Labels == nil {
		return time.Time{}
	}
	return db.GetLoadBalancerStagedConfigAt(*loadBalancer.Metadata.Labels)
}

func (c *controller) observeLoadBalancerAgentState(loadBalancer api.LoadBalancer, targetIP, desiredHash string, stagedAt time.Time) bool {
	loadBalancerID := api.LoadBalancerID(loadBalancer)
	state, err := readLoadBalancerAgentState(targetIP, loadBalancerPrivateKeyPath)
	if err != nil {
		failures, labelErr := c.recordLoadBalancerAgentStateReadFailure(loadBalancerID)
		if labelErr != nil {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, labelErr.Error())
			return true
		}
		message := fmt.Sprintf("load balancer agent state unavailable (%d/%d): %v", failures, loadBalancerAgentStateReadMaxFailures, err)
		if failures >= loadBalancerAgentStateReadMaxFailures {
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, message)
			return true
		}
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, message)
		return true
	}
	successes, err := c.recordLoadBalancerAgentStateReadSuccess(loadBalancerID)
	if err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return true
	}
	if strings.TrimSpace(state.LastError) != "" {
		_ = c.resetLoadBalancerAgentStateReadSuccesses(loadBalancerID)
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, state.LastError)
		return true
	}
	if strings.TrimSpace(state.LastAppliedHash) != strings.TrimSpace(desiredHash) {
		_ = c.resetLoadBalancerAgentStateReadSuccesses(loadBalancerID)
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, "waiting for load balancer agent to apply desired config")
		return true
	}
	if !stagedAt.IsZero() {
		if state.LastAppliedAt.IsZero() || state.LastAppliedAt.UTC().Before(stagedAt.UTC()) {
			_ = c.resetLoadBalancerAgentStateReadSuccesses(loadBalancerID)
			_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, "waiting for newer load balancer agent apply result")
			return true
		}
	}
	if err := c.updateLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		db.SetLoadBalancerAppliedConfigHash(labels, desiredHash)
		db.SetLoadBalancerStagedConfigHash(labels, desiredHash)
		db.SetLoadBalancerStagedConfigAt(labels, stagedAt)
	}); err != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, err.Error())
		return true
	}
	if loadBalancer.Status != nil && loadBalancer.Status.StatusCode == db.LOAD_BALANCER_DEGRADED && successes < loadBalancerAgentStateRecoverySuccessRequired {
		msg := fmt.Sprintf("waiting for consecutive successful load balancer agent checks (%d/%d)", successes, loadBalancerAgentStateRecoverySuccessRequired)
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_DEGRADED, msg)
		return true
	}
	_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_ACTIVE, "")
	return true
}

func (c *controller) resolveLoadBalancerTargetAddress(loadBalancer api.LoadBalancer) (string, error) {
	serverID := strings.TrimSpace(loadBalancerManagedServerID(loadBalancer))
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
	publicIP := strings.TrimSpace(loadBalancer.Spec.BindPublicIpAddress)
	for _, nic := range *server.Spec.NetworkInterface {
		if nic.Address != nil {
			if ip := strings.TrimSpace(*nic.Address); publicIP != "" && ip == publicIP {
				return ip, nil
			}
		}
		if strings.TrimSpace(nic.Networkname) == "host-bridge" && nic.Address != nil {
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

func (c *controller) handleLoadBalancerConfigFailure(loadBalancerID string, err error) {
	if err == nil {
		return
	}
	retries, labelErr := c.incrementLoadBalancerConfigRetries(loadBalancerID)
	if labelErr != nil {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, labelErr.Error())
		return
	}
	message := fmt.Sprintf("load balancer ansible apply failed (%d/%d): %v", retries, loadBalancerAnsibleMaxRetryCount, err)
	if retries >= loadBalancerAnsibleMaxRetryCount {
		_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_FAILED, message)
		return
	}
	_ = c.db.UpdateLoadBalancerStatusWithMessage(loadBalancerID, db.LOAD_BALANCER_CONFIGURING, message)
}

func (c *controller) incrementLoadBalancerConfigRetries(loadBalancerID string) (int, error) {
	next := 0
	err := c.updateLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		next = db.GetLoadBalancerAnsibleRetries(labels) + 1
		db.SetLoadBalancerAnsibleRetries(labels, next)
	})
	return next, err
}

func (c *controller) recordLoadBalancerAgentStateReadFailure(loadBalancerID string) (int, error) {
	next := 0
	err := c.updateLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		next = db.GetLoadBalancerAgentStateReadFailures(labels) + 1
		db.SetLoadBalancerAgentStateReadFailures(labels, next)
		db.SetLoadBalancerAgentStateReadSuccesses(labels, 0)
	})
	return next, err
}

func (c *controller) recordLoadBalancerAgentStateReadSuccess(loadBalancerID string) (int, error) {
	next := 0
	err := c.updateLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		db.SetLoadBalancerAgentStateReadFailures(labels, 0)
		next = db.GetLoadBalancerAgentStateReadSuccesses(labels) + 1
		db.SetLoadBalancerAgentStateReadSuccesses(labels, next)
	})
	return next, err
}

func (c *controller) resetLoadBalancerAgentStateReadSuccesses(loadBalancerID string) error {
	return c.updateLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		db.SetLoadBalancerAgentStateReadSuccesses(labels, 0)
	})
}

func loadBalancerServerName(loadBalancer api.LoadBalancer) string {
	id := strings.TrimSpace(api.LoadBalancerID(loadBalancer))
	if id == "" {
		id = strings.TrimSpace(loadBalancer.Metadata.Name)
	}
	return fmt.Sprintf("lb-%s", id)
}

func loadBalancerControllerSettingsFromEnv() {
	intervalSeconds := readPositiveIntEnv("MARMOT_LB_CONTROLLER_INTERVAL_SECONDS", defaultLoadBalancerControllerIntervalSeconds)
	loadBalancerControllerInterval = time.Duration(intervalSeconds) * time.Second
	loadBalancerAgentStateReadMaxFailures = readPositiveIntEnv("MARMOT_LB_AGENT_STATE_READ_MAX_FAILURES", defaultLoadBalancerAgentStateReadMaxFailures)
	loadBalancerAgentStateRecoverySuccessRequired = readPositiveIntEnv("MARMOT_LB_AGENT_RECOVERY_SUCCESS_REQUIRED", defaultLoadBalancerAgentStateRecoverySuccessRequired)
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