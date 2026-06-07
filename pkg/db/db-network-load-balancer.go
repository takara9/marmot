package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

const (
	NETWORK_LOAD_BALANCER_PENDING      = 0
	NETWORK_LOAD_BALANCER_ACTIVE       = 1
	NETWORK_LOAD_BALANCER_DEGRADED     = 2
	NETWORK_LOAD_BALANCER_FAILED       = 3
	NETWORK_LOAD_BALANCER_DELETING     = 4
	NETWORK_LOAD_BALANCER_PROVISIONING = 5
	NETWORK_LOAD_BALANCER_CONFIGURING  = 6

	NetworkLoadBalancerLabelManagedServerID         = "networkLoadBalancerServerId"
	NetworkLoadBalancerLabelManagedBy               = "managedBy"
	NetworkLoadBalancerLabelManagedByValue          = "network-load-balancer-controller"
	NetworkLoadBalancerServerLabelID                = "networkLoadBalancerId"
	NetworkLoadBalancerServerLabelRole              = "role"
	NetworkLoadBalancerServerLabelRoleValue         = "network-load-balancer"
	NetworkLoadBalancerLabelAnsibleRetries          = "ansibleRetries"
	NetworkLoadBalancerLabelAppliedConfig           = "appliedConfigHash"
	NetworkLoadBalancerLabelStagedConfig            = "stagedConfigHash"
	NetworkLoadBalancerLabelStagedConfigAt          = "stagedConfigAt"
	NetworkLoadBalancerLabelAgentStateReadFailures  = "agentStateReadFailures"
	NetworkLoadBalancerLabelAgentStateReadSuccesses = "agentStateReadSuccesses"
)

var NetworkLoadBalancerStatus = map[int]string{
	0: "PENDING",
	1: "ACTIVE",
	2: "DEGRADED",
	3: "FAILED",
	4: "DELETING",
	5: "PROVISIONING",
	6: "CONFIGURING",
}

func SetNetworkLoadBalancerManagedServerID(labels map[string]interface{}, serverID string) {
	if labels == nil {
		return
	}
	labels[NetworkLoadBalancerLabelManagedServerID] = strings.TrimSpace(serverID)
	labels[NetworkLoadBalancerLabelManagedBy] = NetworkLoadBalancerLabelManagedByValue
}

func GetNetworkLoadBalancerManagedServerID(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[NetworkLoadBalancerLabelManagedServerID].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func SetNetworkLoadBalancerAnsibleRetries(labels map[string]interface{}, retries int) {
	if labels == nil {
		return
	}
	labels[NetworkLoadBalancerLabelAnsibleRetries] = retries
}

func GetNetworkLoadBalancerAnsibleRetries(labels map[string]interface{}) int {
	if labels == nil {
		return 0
	}
	switch val := labels[NetworkLoadBalancerLabelAnsibleRetries].(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return 0
	}
}

func SetNetworkLoadBalancerAppliedConfigHash(labels map[string]interface{}, hash string) {
	if labels == nil {
		return
	}
	labels[NetworkLoadBalancerLabelAppliedConfig] = strings.TrimSpace(hash)
}

func GetNetworkLoadBalancerAppliedConfigHash(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[NetworkLoadBalancerLabelAppliedConfig].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func SetNetworkLoadBalancerStagedConfigHash(labels map[string]interface{}, hash string) {
	if labels == nil {
		return
	}
	labels[NetworkLoadBalancerLabelStagedConfig] = strings.TrimSpace(hash)
}

func GetNetworkLoadBalancerStagedConfigHash(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[NetworkLoadBalancerLabelStagedConfig].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func SetNetworkLoadBalancerStagedConfigAt(labels map[string]interface{}, at time.Time) {
	if labels == nil {
		return
	}
	if at.IsZero() {
		delete(labels, NetworkLoadBalancerLabelStagedConfigAt)
		return
	}
	labels[NetworkLoadBalancerLabelStagedConfigAt] = at.UTC().Format(time.RFC3339Nano)
}

func GetNetworkLoadBalancerStagedConfigAt(labels map[string]interface{}) time.Time {
	if labels == nil {
		return time.Time{}
	}
	raw, ok := labels[NetworkLoadBalancerLabelStagedConfigAt].(string)
	if !ok {
		return time.Time{}
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return at
}

func SetNetworkLoadBalancerAgentStateReadFailures(labels map[string]interface{}, failures int) {
	if labels == nil {
		return
	}
	labels[NetworkLoadBalancerLabelAgentStateReadFailures] = failures
}

func GetNetworkLoadBalancerAgentStateReadFailures(labels map[string]interface{}) int {
	if labels == nil {
		return 0
	}
	switch val := labels[NetworkLoadBalancerLabelAgentStateReadFailures].(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return 0
	}
}

func SetNetworkLoadBalancerAgentStateReadSuccesses(labels map[string]interface{}, successes int) {
	if labels == nil {
		return
	}
	labels[NetworkLoadBalancerLabelAgentStateReadSuccesses] = successes
}

func GetNetworkLoadBalancerAgentStateReadSuccesses(labels map[string]interface{}) int {
	if labels == nil {
		return 0
	}
	switch val := labels[NetworkLoadBalancerLabelAgentStateReadSuccesses].(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return 0
	}
}

func (d *Database) CreateNetworkLoadBalancer(spec api.NetworkLoadBalancer) (api.NetworkLoadBalancer, error) {
	mutex, err := d.LockKey("/lock/network-load-balancer/create")
	if err != nil {
		return api.NetworkLoadBalancer{}, err
	}
	defer d.UnlockKey(mutex)

	current, err := d.GetNetworkLoadBalancers()
	if err != nil && err != ErrNotFound {
		return api.NetworkLoadBalancer{}, err
	}

	name := strings.TrimSpace(spec.Metadata.Name)
	vnet := strings.TrimSpace(spec.Spec.InternalVirtualNetwork)
	publicIP := strings.TrimSpace(spec.Spec.BindPublicIpAddress)
	for _, nlb := range current {
		if strings.TrimSpace(nlb.Metadata.Name) == name && strings.TrimSpace(nlb.Spec.InternalVirtualNetwork) == vnet {
			return api.NetworkLoadBalancer{}, fmt.Errorf("network load balancer with name %q already exists in internalVirtualNetwork %q", name, vnet)
		}
		if publicIP != "" && strings.TrimSpace(nlb.Spec.BindPublicIpAddress) == publicIP {
			return api.NetworkLoadBalancer{}, fmt.Errorf("bindPublicIpAddress %q is already used by another network load balancer", publicIP)
		}
	}

	rec, err := util.DeepCopy(spec)
	if err != nil {
		return api.NetworkLoadBalancer{}, err
	}

	var key string
	for {
		rec.Metadata.Uuid = util.StringPtr(uuid.New().String())
		id := (*rec.Metadata.Uuid)[:5]
		api.SetNetworkLoadBalancerID(&rec, id)
		key = NetworkLoadBalancerPrefix + "/" + id

		var existing api.NetworkLoadBalancer
		_, getErr := d.GetJSON(key, &existing)
		if getErr == ErrNotFound {
			break
		}
		if getErr != nil {
			return api.NetworkLoadBalancer{}, getErr
		}
	}

	now := time.Now()
	rec.Status = &api.Status{
		StatusCode:          NETWORK_LOAD_BALANCER_PENDING,
		Status:              util.StringPtr(NetworkLoadBalancerStatus[NETWORK_LOAD_BALANCER_PENDING]),
		CreationTimeStamp:   util.TimePtr(now),
		LastUpdateTimeStamp: util.TimePtr(now),
	}
	if err := d.PutJSON(key, rec); err != nil {
		return api.NetworkLoadBalancer{}, err
	}

	return rec, nil
}

func (d *Database) GetNetworkLoadBalancers() ([]api.NetworkLoadBalancer, error) {
	result := make([]api.NetworkLoadBalancer, 0)
	resp, err := d.GetByPrefix(NetworkLoadBalancerPrefix)
	if err == ErrNotFound {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	for _, kv := range resp.Kvs {
		var rec api.NetworkLoadBalancer
		if err := json.Unmarshal(kv.Value, &rec); err != nil {
			slog.Error("GetNetworkLoadBalancers() unmarshal failed", "err", err, "key", string(kv.Key))
			continue
		}
		result = append(result, rec)
	}
	return result, nil
}

func (d *Database) GetNetworkLoadBalancerById(id string) (api.NetworkLoadBalancer, error) {
	key := NetworkLoadBalancerPrefix + "/" + id
	var rec api.NetworkLoadBalancer
	_, err := d.GetJSON(key, &rec)
	if err != nil {
		return api.NetworkLoadBalancer{}, err
	}
	return rec, nil
}

func (d *Database) UpdateNetworkLoadBalancerById(id string, spec api.NetworkLoadBalancer) error {
	for {
		err := d.updateNetworkLoadBalancer(id, spec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

func (d *Database) updateNetworkLoadBalancer(id string, spec api.NetworkLoadBalancer) error {
	lockKey := "/lock/network-load-balancer/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	key := NetworkLoadBalancerPrefix + "/" + id
	var rec api.NetworkLoadBalancer
	resp, err := d.GetJSON(key, &rec)
	if err != nil {
		return err
	}
	util.PatchStruct(&rec, spec)
	api.SetNetworkLoadBalancerID(&rec, id)
	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, &rec)
}

func (d *Database) DeleteNetworkLoadBalancerById(id string) error {
	lockKey := "/lock/network-load-balancer/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)
	return d.DeleteJSON(NetworkLoadBalancerPrefix + "/" + id)
}

func (d *Database) SetDeleteTimestampNetworkLoadBalancer(id string) error {
	for {
		rec, err := d.GetNetworkLoadBalancerById(id)
		if err != nil {
			return err
		}
		if rec.Status == nil {
			rec.Status = &api.Status{}
		}
		rec.Status.StatusCode = NETWORK_LOAD_BALANCER_DELETING
		rec.Status.Status = util.StringPtr(NetworkLoadBalancerStatus[NETWORK_LOAD_BALANCER_DELETING])
		rec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
		if rec.Status.DeletionTimeStamp == nil {
			rec.Status.DeletionTimeStamp = util.TimePtr(time.Now())
		}
		rec.Status.Message = nil

		err = d.putNetworkLoadBalancerById(rec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

func (d *Database) UpdateNetworkLoadBalancerStatusWithMessage(id string, status int, message string) error {
	for {
		rec, err := d.GetNetworkLoadBalancerById(id)
		if err != nil {
			return err
		}
		if rec.Status == nil {
			rec.Status = &api.Status{}
		}
		statusChanged := rec.Status.StatusCode != status
		rec.Status.StatusCode = status
		rec.Status.Status = util.StringPtr(NetworkLoadBalancerStatus[status])
		rec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
		trimmed := strings.TrimSpace(message)
		if statusChanged {
			rec.Status.Message = nil
		}
		if trimmed != "" {
			rec.Status.Message = util.StringPtr(trimmed)
		}

		err = d.putNetworkLoadBalancerById(rec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

func (d *Database) putNetworkLoadBalancerById(rec api.NetworkLoadBalancer) error {
	id := api.NetworkLoadBalancerID(rec)
	if strings.TrimSpace(id) == "" {
		return ErrNotFound
	}

	lockKey := "/lock/network-load-balancer/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	key := NetworkLoadBalancerPrefix + "/" + id
	var current api.NetworkLoadBalancer
	resp, err := d.GetJSON(key, &current)
	if err != nil {
		return err
	}
	api.SetNetworkLoadBalancerID(&rec, id)
	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, &rec)
}
