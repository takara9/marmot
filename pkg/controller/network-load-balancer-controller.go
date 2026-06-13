package controller

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
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
	defaultNetworkLoadBalancerControllerIntervalSeconds = 15
	defaultNetworkLoadBalancerApplyRetryMax             = 3
	defaultNetworkLoadBalancerApplyRetryBackoffSeconds  = 10
)

var (
	networkLoadBalancerControllerInterval       = time.Duration(defaultNetworkLoadBalancerControllerIntervalSeconds) * time.Second
	networkLoadBalancerApplyRetryMax            = defaultNetworkLoadBalancerApplyRetryMax
	networkLoadBalancerApplyRetryBackoffSeconds = defaultNetworkLoadBalancerApplyRetryBackoffSeconds
)

// StartNetworkLoadBalancerController starts controller loop for NetworkLoadBalancer resources.
func StartNetworkLoadBalancerController(node string, etcdUrl string) (*controller, error) {
	var c controller
	var err error
	networkLoadBalancerControllerSettingsFromEnv()

	c.deletionDelay = 15 * time.Second
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	ticker := time.NewTicker(networkLoadBalancerControllerInterval)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.networkLoadBalancerControllerLoop()
			case <-c.stopChan:
				slog.Debug("ネットワークロードバランサーコントローラー停止")
				return
			}
		}
	}()

	return &c, nil
}

func networkLoadBalancerControllerSettingsFromEnv() {
	if value := strings.TrimSpace(os.Getenv("MARMOT_NLB_APPLY_RETRY_MAX")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			networkLoadBalancerApplyRetryMax = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("MARMOT_NLB_APPLY_RETRY_BACKOFF_SECONDS")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 0 {
			networkLoadBalancerApplyRetryBackoffSeconds = parsed
		}
	}
}

func (c *controller) networkLoadBalancerControllerLoop() {
	slog.Debug("ネットワークロードバランサーコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))

	items, err := c.db.GetNetworkLoadBalancers()
	if err != nil {
		slog.Error("GetNetworkLoadBalancers() failed", "err", err)
		return
	}

	for _, item := range items {
		id := api.NetworkLoadBalancerID(item)
		if ok, assignedNode, reason := evaluateNodeAssignment(&item.Metadata, c.marmot.NodeName); !ok {
			slog.Debug("別ノード割当のネットワークロードバランサーをスキップ", "id", id, "name", item.Metadata.Name, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			continue
		}

		if item.Status == nil || item.Status.StatusCode != db.NETWORK_LOAD_BALANCER_DELETING {
			missingServer, err := c.isNetworkLoadBalancerManagedServerMissing(item)
			if err != nil {
				slog.Warn("failed to validate network load balancer managed server", "id", id, "err", err)
				continue
			}
			if missingServer {
				c.deleteNetworkLoadBalancerForMissingServer(item)
				continue
			}
		}

		if item.Status == nil {
			_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(id, db.NETWORK_LOAD_BALANCER_PENDING, "")
			continue
		}

		if item.Status.DeletionTimeStamp != nil && time.Since(*item.Status.DeletionTimeStamp) > c.deletionDelay {
			if item.Status.StatusCode != db.NETWORK_LOAD_BALANCER_DELETING {
				_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(id, db.NETWORK_LOAD_BALANCER_DELETING, "")
				continue
			}
		}

		switch item.Status.StatusCode {
		case db.NETWORK_LOAD_BALANCER_PENDING:
			c.reconcileNetworkLoadBalancerPending(item)
		case db.NETWORK_LOAD_BALANCER_PROVISIONING:
			c.reconcileNetworkLoadBalancerProvisioning(item)
		case db.NETWORK_LOAD_BALANCER_CONFIGURING:
			c.reconcileNetworkLoadBalancerConfiguring(item)
		case db.NETWORK_LOAD_BALANCER_ACTIVE:
			c.reconcileNetworkLoadBalancerActive(item)
		case db.NETWORK_LOAD_BALANCER_DELETING:
			c.reconcileNetworkLoadBalancerDeleting(item)
		case db.NETWORK_LOAD_BALANCER_FAILED:
			slog.Debug("ネットワークロードバランサー状態を監視", "id", id, "statusCode", item.Status.StatusCode)
		default:
			slog.Warn("不明なネットワークロードバランサー状態", "id", id, "statusCode", item.Status.StatusCode)
		}
	}
}

func (c *controller) isNetworkLoadBalancerManagedServerMissing(loadBalancer api.NetworkLoadBalancer) (bool, error) {
	serverID := strings.TrimSpace(networkLoadBalancerManagedServerID(loadBalancer))
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

func (c *controller) deleteNetworkLoadBalancerForMissingServer(loadBalancer api.NetworkLoadBalancer) {
	loadBalancerID := api.NetworkLoadBalancerID(loadBalancer)
	c.cleanupNetworkLoadBalancerPlaybook(loadBalancerID)
	if err := c.db.DeleteNetworkLoadBalancerById(loadBalancerID); err != nil {
		slog.Warn("DeleteNetworkLoadBalancerById() failed while auto-deleting network load balancer", "id", loadBalancerID, "err", err)
		return
	}
	slog.Debug("network load balancer deleted because managed server no longer exists", "id", loadBalancerID)
}

func (c *controller) reconcileNetworkLoadBalancerPending(loadBalancer api.NetworkLoadBalancer) {
	loadBalancerID := api.NetworkLoadBalancerID(loadBalancer)

	if err := validateGatewayInternalNetwork(c.db, loadBalancer.Spec.InternalVirtualNetwork); err != nil {
		if isRetryableNetworkLoadBalancerPendingError(err) {
			slog.Warn("validateGatewayInternalNetwork() failed with retryable error; keep network load balancer pending", "id", loadBalancerID, "err", err)
			retryMsg := fmt.Sprintf("ネットワークロードバランサーのプロビジョニング待機中（依存関係の準備待ち）: %v", err)
			_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_PENDING, retryMsg)
			return
		}
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
		return
	}

	serverID, err := c.ensureNetworkLoadBalancerServerEntry(loadBalancer)
	if err != nil {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
		return
	}
	if err := c.ensureNetworkLoadBalancerManagedServerLabel(loadBalancerID, serverID); err != nil {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
		return
	}
	if err := c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_PROVISIONING, "network load balancer VM provisioning started"); err != nil {
		slog.Warn("UpdateNetworkLoadBalancerStatusWithMessage() failed", "id", loadBalancerID, "err", err)
	}
}

func (c *controller) reconcileNetworkLoadBalancerProvisioning(loadBalancer api.NetworkLoadBalancer) {
	loadBalancerID := api.NetworkLoadBalancerID(loadBalancer)
	serverID := networkLoadBalancerManagedServerID(loadBalancer)
	if strings.TrimSpace(serverID) == "" {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_PENDING, "network load balancer server reference is missing")
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_PENDING, "network load balancer server not found, recreating")
			return
		}
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
		return
	}
	if server.Status == nil {
		return
	}

	switch server.Status.StatusCode {
	case db.SERVER_RUNNING:
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_CONFIGURING, "network load balancer VM running, applying ansible")
	case db.SERVER_ERROR:
		msg := "network load balancer server entered error state"
		if server.Status.Message != nil && strings.TrimSpace(*server.Status.Message) != "" {
			msg = strings.TrimSpace(*server.Status.Message)
		}
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, msg)
	}
}

func (c *controller) reconcileNetworkLoadBalancerConfiguring(loadBalancer api.NetworkLoadBalancer) {
	loadBalancerID := api.NetworkLoadBalancerID(loadBalancer)
	serverID := networkLoadBalancerManagedServerID(loadBalancer)
	if strings.TrimSpace(serverID) == "" {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_PENDING, "network load balancer server reference is missing")
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_PENDING, "network load balancer server not found, recreating")
			return
		}
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
		return
	}
	if server.Status == nil || server.Status.StatusCode != db.SERVER_RUNNING {
		return
	}

	if !c.shouldRetryNetworkLoadBalancerApply(loadBalancer) {
		return
	}

	targetIP, err := c.resolveNetworkLoadBalancerTargetAddress(loadBalancer)
	if err != nil {
		c.handleNetworkLoadBalancerConfigFailure(loadBalancerID, loadBalancer, "", err)
		return
	}
	listenerBackends, err := c.resolveNetworkLoadBalancerListenerBackends(loadBalancer)
	if err != nil {
		c.handleNetworkLoadBalancerConfigFailure(loadBalancerID, loadBalancer, "", err)
		return
	}
	if msg := networkLoadBalancerBackendAvailabilityMessage(loadBalancer, listenerBackends); msg != "" {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_DEGRADED, msg)
		return
	}

	playbookPath := networkLoadBalancerPlaybookPath(loadBalancerID)
	data, err := buildNetworkLoadBalancerPlaybookData(loadBalancer, listenerBackends)
	if err != nil {
		c.handleNetworkLoadBalancerConfigFailure(loadBalancerID, loadBalancer, "", err)
		return
	}
	configHash, err := desiredNetworkLoadBalancerConfigHash(loadBalancer, listenerBackends)
	if err != nil {
		c.handleNetworkLoadBalancerConfigFailure(loadBalancerID, loadBalancer, "", err)
		return
	}
	if networkLoadBalancerStagedConfigHash(loadBalancer) == configHash && networkLoadBalancerAppliedConfigHash(loadBalancer) == configHash {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_ACTIVE, "")
		return
	}

	if err := renderNetworkLoadBalancerPlaybook(playbookPath, data); err != nil {
		c.handleNetworkLoadBalancerConfigFailure(loadBalancerID, loadBalancer, configHash, err)
		return
	}
	if err := runNetworkLoadBalancerPlaybook(playbookPath, targetIP, networkLoadBalancerPrivateKeyPath); err != nil {
		c.handleNetworkLoadBalancerConfigFailure(loadBalancerID, loadBalancer, configHash, err)
		return
	}
	if err := c.updateNetworkLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		db.SetNetworkLoadBalancerAnsibleRetries(labels, 0)
		db.SetNetworkLoadBalancerAppliedConfigHash(labels, configHash)
		db.SetNetworkLoadBalancerStagedConfigHash(labels, configHash)
		db.SetNetworkLoadBalancerStagedConfigAt(labels, time.Now().UTC())
		db.SetNetworkLoadBalancerAgentStateReadFailures(labels, 0)
		db.SetNetworkLoadBalancerAgentStateReadSuccesses(labels, 0)
	}); err != nil {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
		return
	}
	_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_ACTIVE, "")
}

func (c *controller) reconcileNetworkLoadBalancerActive(loadBalancer api.NetworkLoadBalancer) {
	loadBalancerID := api.NetworkLoadBalancerID(loadBalancer)
	serverID := networkLoadBalancerManagedServerID(loadBalancer)
	if strings.TrimSpace(serverID) == "" {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_PENDING, "network load balancer server reference is missing")
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_PENDING, "network load balancer server disappeared")
			return
		}
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
		return
	}
	if server.Status != nil && server.Status.StatusCode == db.SERVER_ERROR {
		msg := "network load balancer server entered error state"
		if server.Status.Message != nil && strings.TrimSpace(*server.Status.Message) != "" {
			msg = strings.TrimSpace(*server.Status.Message)
		}
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, msg)
		return
	}

	listenerBackends, err := c.resolveNetworkLoadBalancerListenerBackends(loadBalancer)
	if err != nil {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_DEGRADED, err.Error())
		return
	}
	if msg := networkLoadBalancerBackendAvailabilityMessage(loadBalancer, listenerBackends); msg != "" {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_DEGRADED, msg)
		return
	}

	desiredHash, err := desiredNetworkLoadBalancerConfigHash(loadBalancer, listenerBackends)
	if err != nil {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
		return
	}

	if networkLoadBalancerAppliedConfigHash(loadBalancer) != desiredHash || networkLoadBalancerStagedConfigHash(loadBalancer) != desiredHash {
		if err := c.updateNetworkLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
			db.SetNetworkLoadBalancerAnsibleRetries(labels, 0)
			db.SetNetworkLoadBalancerStagedConfigHash(labels, desiredHash)
			db.SetNetworkLoadBalancerStagedConfigAt(labels, time.Now().UTC())
		}); err != nil {
			_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
			return
		}
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_CONFIGURING, "network load balancer configuration drift detected")
		return
	}

	_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_ACTIVE, "")
}

func (c *controller) reconcileNetworkLoadBalancerDeleting(loadBalancer api.NetworkLoadBalancer) {
	loadBalancerID := api.NetworkLoadBalancerID(loadBalancer)
	c.cleanupNetworkLoadBalancerPlaybook(loadBalancerID)
	serverID := networkLoadBalancerManagedServerID(loadBalancer)
	if strings.TrimSpace(serverID) == "" {
		if err := c.db.DeleteNetworkLoadBalancerById(loadBalancerID); err != nil {
			slog.Warn("DeleteNetworkLoadBalancerById() failed", "id", loadBalancerID, "err", err)
		}
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			if err := c.db.DeleteNetworkLoadBalancerById(loadBalancerID); err != nil {
				slog.Warn("DeleteNetworkLoadBalancerById() failed", "id", loadBalancerID, "err", err)
			}
			return
		}
		slog.Warn("GetServerById() failed while deleting network load balancer", "id", loadBalancerID, "serverId", serverID, "err", err)
		return
	}

	if server.Status == nil || server.Status.StatusCode != db.SERVER_DELETING {
		if err := c.db.SetDeleteTimestamp(serverID); err != nil {
			slog.Warn("SetDeleteTimestamp() failed", "id", loadBalancerID, "serverId", serverID, "err", err)
		}
	}
}

func (c *controller) cleanupNetworkLoadBalancerPlaybook(loadBalancerID string) {
	path := networkLoadBalancerPlaybookPath(loadBalancerID)
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("failed to remove network load balancer playbook", "id", loadBalancerID, "path", path, "err", err)
	}
}

func (c *controller) ensureNetworkLoadBalancerManagedServerLabel(loadBalancerID string, serverID string) error {
	loadBalancer, err := c.db.GetNetworkLoadBalancerById(loadBalancerID)
	if err != nil {
		return err
	}
	if loadBalancer.Metadata.Labels == nil {
		labels := map[string]interface{}{}
		loadBalancer.Metadata.Labels = &labels
	}
	labels := *loadBalancer.Metadata.Labels
	if db.GetNetworkLoadBalancerManagedServerID(labels) == strings.TrimSpace(serverID) {
		return nil
	}
	db.SetNetworkLoadBalancerManagedServerID(labels, serverID)
	loadBalancer.Metadata.Labels = &labels
	return c.db.UpdateNetworkLoadBalancerById(loadBalancerID, loadBalancer)
}

func (c *controller) updateNetworkLoadBalancerLabels(loadBalancerID string, mutate func(labels map[string]interface{})) error {
	loadBalancer, err := c.db.GetNetworkLoadBalancerById(loadBalancerID)
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
	return c.db.UpdateNetworkLoadBalancerById(loadBalancerID, loadBalancer)
}

func (c *controller) ensureNetworkLoadBalancerServerEntry(loadBalancer api.NetworkLoadBalancer) (string, error) {
	if serverID := networkLoadBalancerManagedServerID(loadBalancer); strings.TrimSpace(serverID) != "" {
		if _, err := c.db.GetServerById(serverID); err == nil {
			return serverID, nil
		}
	}

	serverName := networkLoadBalancerServerName(loadBalancer)
	if existing, err := c.findServerByName(serverName); err == nil {
		return api.ServerID(existing), nil
	}

	serverSpec, err := c.buildNetworkLoadBalancerServerSpec(loadBalancer, serverName)
	if err != nil {
		return "", err
	}

	created, err := c.db.MakeServerEntry(serverSpec)
	if err != nil {
		return "", err
	}

	return api.ServerID(created), nil
}

func (c *controller) buildNetworkLoadBalancerServerSpec(loadBalancer api.NetworkLoadBalancer, serverName string) (api.Server, error) {
	publicIP := strings.TrimSpace(loadBalancer.Spec.BindPublicIpAddress)
	if publicIP == "" {
		return api.Server{}, fmt.Errorf("network load balancer bindPublicIpAddress is empty")
	}
	internalNetwork := strings.TrimSpace(loadBalancer.Spec.InternalVirtualNetwork)
	if internalNetwork == "" {
		return api.Server{}, fmt.Errorf("network load balancer internalVirtualNetwork is empty")
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
		db.NetworkLoadBalancerServerLabelID:   api.NetworkLoadBalancerID(loadBalancer),
		db.NetworkLoadBalancerServerLabelRole: db.NetworkLoadBalancerServerLabelRoleValue,
		db.NetworkLoadBalancerLabelManagedBy:  db.NetworkLoadBalancerLabelManagedByValue,
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

func networkLoadBalancerManagedServerID(loadBalancer api.NetworkLoadBalancer) string {
	if loadBalancer.Metadata.Labels == nil {
		return ""
	}
	return db.GetNetworkLoadBalancerManagedServerID(*loadBalancer.Metadata.Labels)
}

func networkLoadBalancerAppliedConfigHash(loadBalancer api.NetworkLoadBalancer) string {
	if loadBalancer.Metadata.Labels == nil {
		return ""
	}
	return db.GetNetworkLoadBalancerAppliedConfigHash(*loadBalancer.Metadata.Labels)
}

func networkLoadBalancerStagedConfigHash(loadBalancer api.NetworkLoadBalancer) string {
	if loadBalancer.Metadata.Labels == nil {
		return ""
	}
	return db.GetNetworkLoadBalancerStagedConfigHash(*loadBalancer.Metadata.Labels)
}

func networkLoadBalancerStagedConfigAt(loadBalancer api.NetworkLoadBalancer) time.Time {
	if loadBalancer.Metadata.Labels == nil {
		return time.Time{}
	}
	return db.GetNetworkLoadBalancerStagedConfigAt(*loadBalancer.Metadata.Labels)
}

func networkLoadBalancerServerName(loadBalancer api.NetworkLoadBalancer) string {
	name := strings.TrimSpace(loadBalancer.Metadata.Name)
	if name == "" {
		name = strings.TrimSpace(api.NetworkLoadBalancerID(loadBalancer))
	}
	if name == "" {
		return "network-load-balancer"
	}
	return "nlb-" + name
}

func (c *controller) shouldRetryNetworkLoadBalancerApply(loadBalancer api.NetworkLoadBalancer) bool {
	stagedAt := networkLoadBalancerStagedConfigAt(loadBalancer)
	if stagedAt.IsZero() {
		return true
	}
	if networkLoadBalancerApplyRetryBackoffSeconds <= 0 {
		return true
	}
	return time.Since(stagedAt) >= time.Duration(networkLoadBalancerApplyRetryBackoffSeconds)*time.Second
}

func (c *controller) handleNetworkLoadBalancerConfigFailure(loadBalancerID string, loadBalancer api.NetworkLoadBalancer, desiredHash string, err error) {
	if err == nil {
		return
	}
	retries, labelErr := c.incrementNetworkLoadBalancerConfigRetries(loadBalancerID)
	if labelErr != nil {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, labelErr.Error())
		return
	}
	if desiredHash != "" {
		_ = c.updateNetworkLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
			db.SetNetworkLoadBalancerStagedConfigHash(labels, desiredHash)
			db.SetNetworkLoadBalancerStagedConfigAt(labels, time.Now().UTC())
		})
	}
	message := fmt.Sprintf("network load balancer ansible apply failed (%d/%d): %v", retries, networkLoadBalancerApplyRetryMax, err)
	if retries >= networkLoadBalancerApplyRetryMax {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, message)
		return
	}
	_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_CONFIGURING, message)
}

func (c *controller) incrementNetworkLoadBalancerConfigRetries(loadBalancerID string) (int, error) {
	next := 0
	err := c.updateNetworkLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
		next = db.GetNetworkLoadBalancerAnsibleRetries(labels) + 1
		db.SetNetworkLoadBalancerAnsibleRetries(labels, next)
	})
	return next, err
}

func (c *controller) resolveNetworkLoadBalancerListenerBackends(loadBalancer api.NetworkLoadBalancer) (map[string][]networkLoadBalancerBackendServer, error) {
	servers, err := c.db.GetServers()
	if err != nil {
		return nil, err
	}

	internalNetwork := strings.TrimSpace(loadBalancer.Spec.InternalVirtualNetwork)
	result := make(map[string][]networkLoadBalancerBackendServer, len(loadBalancer.Spec.Listeners))
	for _, listener := range loadBalancer.Spec.Listeners {
		listenerName := strings.TrimSpace(listener.Name)
		if listenerName == "" {
			continue
		}
		items := make([]networkLoadBalancerBackendServer, 0)
		for _, server := range servers {
			if server.Status == nil || server.Status.StatusCode != db.SERVER_RUNNING {
				continue
			}
			if !serverMatchesNetworkLoadBalancerSelector(server, listener.BackendSelector.MatchLabels) {
				continue
			}
			ip := networkLoadBalancerServerAddressInNetwork(server, internalNetwork)
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
			items = append(items, networkLoadBalancerBackendServer{Name: name, IP: ip})
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

func serverMatchesNetworkLoadBalancerSelector(server api.Server, matchLabels map[string]string) bool {
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

func networkLoadBalancerServerAddressInNetwork(server api.Server, networkName string) string {
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

func (c *controller) resolveNetworkLoadBalancerTargetAddress(loadBalancer api.NetworkLoadBalancer) (string, error) {
	serverID := strings.TrimSpace(networkLoadBalancerManagedServerID(loadBalancer))
	if serverID == "" {
		return "", fmt.Errorf("network load balancer server reference is missing")
	}
	server, err := c.db.GetServerById(serverID)
	if err != nil {
		return "", err
	}
	if server.Spec.NetworkInterface == nil {
		return "", fmt.Errorf("network load balancer server has no network interface")
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
	return "", fmt.Errorf("network load balancer public target address is missing")
}

func isRetryableNetworkLoadBalancerPendingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "internalvirtualnetwork") && strings.Contains(msg, "not found")
}
