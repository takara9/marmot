package controller

import (
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/takara9/marmot/pkg/nlb"
)

const defaultNetworkLoadBalancerControllerInterval = 15 * time.Second

const defaultNetworkLoadBalancerApplyRetryMax = 3
const defaultNetworkLoadBalancerApplyRetryBackoff = 10 * time.Second

var networkLoadBalancerApplyRetryMax = defaultNetworkLoadBalancerApplyRetryMax
var networkLoadBalancerApplyRetryBackoff = defaultNetworkLoadBalancerApplyRetryBackoff

const (
	networkLoadBalancerMsgCodeBackendEmpty  = "NLB_BACKEND_EMPTY"
	networkLoadBalancerMsgCodeConfigDrift   = "NLB_CONFIG_DRIFT"
	networkLoadBalancerMsgCodeApplyDeferred = "NLB_APPLY_DEFERRED"
	networkLoadBalancerMsgCodeApplyRetry    = "NLB_APPLY_RETRY"
	networkLoadBalancerMsgCodeApplyFailed   = "NLB_APPLY_FAILED"
	networkLoadBalancerMsgCodeCleanupRetry  = "NLB_CLEANUP_RETRY"
)

// StartNetworkLoadBalancerController starts controller loop for network-load-balancer resources.
func StartNetworkLoadBalancerController(node string, etcdURL string) (*controller, error) {
	var c controller
	var err error
	networkLoadBalancerControllerSettingsFromEnv()

	c.deletionDelay = 15 * time.Second
	c.marmot, err = marmotd.NewMarmot(node, etcdURL)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	ticker := time.NewTicker(defaultNetworkLoadBalancerControllerInterval)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.networkLoadBalancerControllerLoop()
			case <-c.stopChan:
				slog.Info("network load balancer controller stopped")
				return
			}
		}
	}()

	return &c, nil
}

func networkLoadBalancerControllerSettingsFromEnv() {
	networkLoadBalancerApplyRetryMax = defaultNetworkLoadBalancerApplyRetryMax
	networkLoadBalancerApplyRetryBackoff = defaultNetworkLoadBalancerApplyRetryBackoff

	if raw := strings.TrimSpace(os.Getenv("MARMOT_NLB_APPLY_RETRY_MAX")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			networkLoadBalancerApplyRetryMax = value
		} else {
			slog.Warn("ignore invalid MARMOT_NLB_APPLY_RETRY_MAX", "value", raw, "err", err)
		}
	}

	if raw := strings.TrimSpace(os.Getenv("MARMOT_NLB_APPLY_RETRY_BACKOFF_SECONDS")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			networkLoadBalancerApplyRetryBackoff = time.Duration(value) * time.Second
		} else {
			slog.Warn("ignore invalid MARMOT_NLB_APPLY_RETRY_BACKOFF_SECONDS", "value", raw, "err", err)
		}
	}
}

func (c *controller) networkLoadBalancerControllerLoop() {
	slog.Debug("network load balancer controller loop", "time", time.Now().Format("2006-01-02 15:04:05"))

	items, err := c.db.GetNetworkLoadBalancers()
	if err != nil {
		slog.Error("GetNetworkLoadBalancers() failed", "err", err)
		return
	}

	for _, item := range items {
		id := api.NetworkLoadBalancerID(item)
		if ok, assignedNode, reason := evaluateNodeAssignment(&item.Metadata, c.marmot.NodeName); !ok {
			slog.Debug("skip network load balancer assigned to another node", "id", id, "name", item.Metadata.Name, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			continue
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
		case db.NETWORK_LOAD_BALANCER_PENDING, db.NETWORK_LOAD_BALANCER_FAILED, db.NETWORK_LOAD_BALANCER_ACTIVE, db.NETWORK_LOAD_BALANCER_CONFIGURING:
			c.reconcileNetworkLoadBalancerDesiredState(item)
		case db.NETWORK_LOAD_BALANCER_DELETING:
			c.reconcileNetworkLoadBalancerDeleting(item)
		default:
			slog.Warn("unknown network load balancer status", "id", id, "statusCode", item.Status.StatusCode)
		}
	}
}

func (c *controller) reconcileNetworkLoadBalancerDeleting(loadBalancer api.NetworkLoadBalancer) {
	loadBalancerID := api.NetworkLoadBalancerID(loadBalancer)
	if err := runNetworkLoadBalancerCleanup(loadBalancer.Spec, networkLoadBalancerChainPrefix(loadBalancerID)); err != nil {
		slog.Warn("runNetworkLoadBalancerCleanup() failed", "id", loadBalancerID, "err", err)
		msg := networkLoadBalancerStatusMessage(networkLoadBalancerMsgCodeCleanupRetry, fmt.Sprintf("network load balancer cleanup failed, retrying: %v", err))
		if statusErr := c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_DELETING, msg); statusErr != nil {
			slog.Warn("UpdateNetworkLoadBalancerStatusWithMessage() failed", "id", loadBalancerID, "err", statusErr)
		}
		return
	}
	if err := c.db.DeleteNetworkLoadBalancerById(loadBalancerID); err != nil {
		slog.Warn("DeleteNetworkLoadBalancerById() failed", "id", loadBalancerID, "err", err)
	}
}

func (c *controller) reconcileNetworkLoadBalancerDesiredState(loadBalancer api.NetworkLoadBalancer) {
	loadBalancerID := api.NetworkLoadBalancerID(loadBalancer)
	listenerBackends, err := c.resolveNetworkLoadBalancerListenerBackends(loadBalancer)
	if err != nil {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
		return
	}
	if msg := networkLoadBalancerBackendAvailabilityMessage(loadBalancer, listenerBackends); msg != "" {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, msg)
		return
	}

	planInput := nlb.IPTablesPlanInput{
		BindPublicIP: strings.TrimSpace(loadBalancer.Spec.BindPublicIpAddress),
		ChainPrefix:  networkLoadBalancerChainPrefix(loadBalancerID),
		RemoteCIDR:   strings.TrimSpace(loadBalancer.Spec.RemoteCIDR),
		Listeners:    make([]nlb.Listener, 0, len(loadBalancer.Spec.Listeners)),
	}
	for _, listener := range loadBalancer.Spec.Listeners {
		name := strings.TrimSpace(listener.Name)
		if name == "" {
			continue
		}
		planInput.Listeners = append(planInput.Listeners, nlb.Listener{
			Name:               name,
			Protocol:           strings.ToLower(strings.TrimSpace(listener.Protocol)),
			VipPort:            listener.VipPort,
			SessionPersistence: listener.SessionPersistence != nil && listener.SessionPersistence.Enabled,
			Backends:           listenerBackends[name],
		})
	}

	script, err := nlb.BuildIPTablesRestoreScript(planInput)
	if err != nil {
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
		return
	}

	desiredHash := stablePlanHash(script)
	currentHash := networkLoadBalancerAppliedConfigHash(loadBalancer)
	currentRetries := networkLoadBalancerApplyRetries(loadBalancer)
	lastFailedAt := networkLoadBalancerApplyFailedAt(loadBalancer)
	if loadBalancer.Status != nil && loadBalancer.Status.StatusCode == db.NETWORK_LOAD_BALANCER_ACTIVE {
		if currentHash == desiredHash {
			slog.Debug("network load balancer plan validated", "id", loadBalancerID, "planHash", desiredHash)
			return
		}
		if currentRetries != 0 {
			if err := c.updateNetworkLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
				db.SetNetworkLoadBalancerApplyRetries(labels, 0)
				db.SetNetworkLoadBalancerApplyFailedAt(labels, time.Time{})
			}); err != nil {
				_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
				return
			}
		}
		if err := c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_CONFIGURING, networkLoadBalancerStatusMessage(networkLoadBalancerMsgCodeConfigDrift, "network load balancer configuration drift detected")); err != nil {
			slog.Warn("UpdateNetworkLoadBalancerStatusWithMessage() failed", "id", loadBalancerID, "err", err)
		}
		return
	}

	if currentRetries > 0 && !lastFailedAt.IsZero() {
		nextRetryAt := lastFailedAt.Add(networkLoadBalancerApplyRetryBackoff)
		if time.Now().Before(nextRetryAt) {
			msg := networkLoadBalancerStatusMessage(networkLoadBalancerMsgCodeApplyDeferred, fmt.Sprintf("network load balancer apply retry is deferred until %s (attempt %d/%d)", nextRetryAt.Format(time.RFC3339), currentRetries+1, networkLoadBalancerApplyRetryMax))
			_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_CONFIGURING, msg)
			return
		}
	}

	if err := runNetworkLoadBalancerApply(script); err != nil {
		nextRetries := currentRetries + 1
		if labelErr := c.updateNetworkLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
			db.SetNetworkLoadBalancerApplyRetries(labels, nextRetries)
			db.SetNetworkLoadBalancerApplyFailedAt(labels, time.Now().UTC())
		}); labelErr != nil {
			_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, labelErr.Error())
			return
		}

		if nextRetries >= networkLoadBalancerApplyRetryMax {
			msg := networkLoadBalancerStatusMessage(networkLoadBalancerMsgCodeApplyFailed, fmt.Sprintf("network load balancer apply failed after %d retries: %v", nextRetries, err))
			_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, msg)
			return
		}

		msg := networkLoadBalancerStatusMessage(networkLoadBalancerMsgCodeApplyRetry, fmt.Sprintf("network load balancer apply failed, retrying (%d/%d): %v", nextRetries, networkLoadBalancerApplyRetryMax, err))
		_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_CONFIGURING, msg)
		return
	}

	if currentHash != desiredHash || currentRetries != 0 || !lastFailedAt.IsZero() {
		if err := c.updateNetworkLoadBalancerLabels(loadBalancerID, func(labels map[string]interface{}) {
			db.SetNetworkLoadBalancerAppliedConfigHash(labels, desiredHash)
			db.SetNetworkLoadBalancerApplyRetries(labels, 0)
			db.SetNetworkLoadBalancerApplyFailedAt(labels, time.Time{})
		}); err != nil {
			_ = c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_FAILED, err.Error())
			return
		}
	}

	if err := c.db.UpdateNetworkLoadBalancerStatusWithMessage(loadBalancerID, db.NETWORK_LOAD_BALANCER_ACTIVE, ""); err != nil {
		slog.Warn("UpdateNetworkLoadBalancerStatusWithMessage() failed", "id", loadBalancerID, "err", err)
	}
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

func (c *controller) resolveNetworkLoadBalancerListenerBackends(loadBalancer api.NetworkLoadBalancer) (map[string][]nlb.Backend, error) {
	servers, err := c.db.GetServers()
	if err != nil {
		return nil, err
	}

	internalNetwork := strings.TrimSpace(loadBalancer.Spec.InternalVirtualNetwork)
	result := make(map[string][]nlb.Backend, len(loadBalancer.Spec.Listeners))
	for _, listener := range loadBalancer.Spec.Listeners {
		listenerName := strings.TrimSpace(listener.Name)
		if listenerName == "" {
			continue
		}
		items := make([]nlb.Backend, 0)
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
			items = append(items, nlb.Backend{Address: stripCIDRSuffix(ip), Port: listener.BackendPort})
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Address == items[j].Address {
				return items[i].Port < items[j].Port
			}
			return items[i].Address < items[j].Address
		})
		result[listenerName] = items
	}
	return result, nil
}

func networkLoadBalancerBackendAvailabilityMessage(loadBalancer api.NetworkLoadBalancer, listenerBackends map[string][]nlb.Backend) string {
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
	return networkLoadBalancerStatusMessage(networkLoadBalancerMsgCodeBackendEmpty, fmt.Sprintf("no backend matched for listener(s): %s", strings.Join(missing, ",")))
}

func stripCIDRSuffix(ip string) string {
	trimmed := strings.TrimSpace(ip)
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return strings.TrimSpace(trimmed[:idx])
	}
	return trimmed
}

func stablePlanHash(script string) string {
	sum := sha256.Sum256([]byte(script))
	return hex.EncodeToString(sum[:8])
}

func networkLoadBalancerAppliedConfigHash(loadBalancer api.NetworkLoadBalancer) string {
	if loadBalancer.Metadata.Labels == nil {
		return ""
	}
	return db.GetNetworkLoadBalancerAppliedConfigHash(*loadBalancer.Metadata.Labels)
}

func networkLoadBalancerApplyRetries(loadBalancer api.NetworkLoadBalancer) int {
	if loadBalancer.Metadata.Labels == nil {
		return 0
	}
	return db.GetNetworkLoadBalancerApplyRetries(*loadBalancer.Metadata.Labels)
}

func networkLoadBalancerApplyFailedAt(loadBalancer api.NetworkLoadBalancer) time.Time {
	if loadBalancer.Metadata.Labels == nil {
		return time.Time{}
	}
	return db.GetNetworkLoadBalancerApplyFailedAt(*loadBalancer.Metadata.Labels)
}

func networkLoadBalancerChainPrefix(loadBalancerID string) string {
	id := strings.TrimSpace(loadBalancerID)
	if id == "" {
		return "NLB"
	}
	return "NLB_" + id
}

func networkLoadBalancerStatusMessage(code, detail string) string {
	c := strings.TrimSpace(code)
	d := strings.TrimSpace(detail)
	if c == "" {
		return d
	}
	if d == "" {
		return c
	}
	return c + ": " + d
}
