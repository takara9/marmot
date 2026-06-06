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
	LOAD_BALANCER_PENDING      = 0
	LOAD_BALANCER_ACTIVE       = 1
	LOAD_BALANCER_DEGRADED     = 2
	LOAD_BALANCER_FAILED       = 3
	LOAD_BALANCER_DELETING     = 4
	LOAD_BALANCER_PROVISIONING = 5
	LOAD_BALANCER_CONFIGURING  = 6

	LoadBalancerLabelManagedServerID         = "loadBalancerServerId"
	LoadBalancerLabelManagedBy               = "managedBy"
	LoadBalancerLabelManagedByValue          = "load-balancer-controller"
	LoadBalancerServerLabelID                = "loadBalancerId"
	LoadBalancerServerLabelRole              = "role"
	LoadBalancerServerLabelRoleValue         = "load-balancer"
	LoadBalancerLabelAnsibleRetries          = "ansibleRetries"
	LoadBalancerLabelAppliedConfig           = "appliedConfigHash"
	LoadBalancerLabelStagedConfig            = "stagedConfigHash"
	LoadBalancerLabelStagedConfigAt          = "stagedConfigAt"
	LoadBalancerLabelAgentStateReadFailures  = "agentStateReadFailures"
	LoadBalancerLabelAgentStateReadSuccesses = "agentStateReadSuccesses"
)

var LoadBalancerStatus = map[int]string{
	0: "PENDING",
	1: "ACTIVE",
	2: "DEGRADED",
	3: "FAILED",
	4: "DELETING",
	5: "PROVISIONING",
	6: "CONFIGURING",
}

func SetLoadBalancerManagedServerID(labels map[string]interface{}, serverID string) {
	if labels == nil {
		return
	}
	labels[LoadBalancerLabelManagedServerID] = strings.TrimSpace(serverID)
	labels[LoadBalancerLabelManagedBy] = LoadBalancerLabelManagedByValue
}

func GetLoadBalancerManagedServerID(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[LoadBalancerLabelManagedServerID].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func SetLoadBalancerAnsibleRetries(labels map[string]interface{}, retries int) {
	if labels == nil {
		return
	}
	labels[LoadBalancerLabelAnsibleRetries] = retries
}

func GetLoadBalancerAnsibleRetries(labels map[string]interface{}) int {
	if labels == nil {
		return 0
	}
	switch val := labels[LoadBalancerLabelAnsibleRetries].(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return 0
	}
}

func SetLoadBalancerAppliedConfigHash(labels map[string]interface{}, hash string) {
	if labels == nil {
		return
	}
	labels[LoadBalancerLabelAppliedConfig] = strings.TrimSpace(hash)
}

func GetLoadBalancerAppliedConfigHash(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[LoadBalancerLabelAppliedConfig].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func SetLoadBalancerStagedConfigHash(labels map[string]interface{}, hash string) {
	if labels == nil {
		return
	}
	labels[LoadBalancerLabelStagedConfig] = strings.TrimSpace(hash)
}

func GetLoadBalancerStagedConfigHash(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[LoadBalancerLabelStagedConfig].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func SetLoadBalancerStagedConfigAt(labels map[string]interface{}, at time.Time) {
	if labels == nil {
		return
	}
	if at.IsZero() {
		delete(labels, LoadBalancerLabelStagedConfigAt)
		return
	}
	labels[LoadBalancerLabelStagedConfigAt] = at.UTC().Format(time.RFC3339Nano)
}

func GetLoadBalancerStagedConfigAt(labels map[string]interface{}) time.Time {
	if labels == nil {
		return time.Time{}
	}
	raw, ok := labels[LoadBalancerLabelStagedConfigAt].(string)
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

func SetLoadBalancerAgentStateReadFailures(labels map[string]interface{}, failures int) {
	if labels == nil {
		return
	}
	labels[LoadBalancerLabelAgentStateReadFailures] = failures
}

func GetLoadBalancerAgentStateReadFailures(labels map[string]interface{}) int {
	if labels == nil {
		return 0
	}
	switch val := labels[LoadBalancerLabelAgentStateReadFailures].(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return 0
	}
}

func SetLoadBalancerAgentStateReadSuccesses(labels map[string]interface{}, successes int) {
	if labels == nil {
		return
	}
	labels[LoadBalancerLabelAgentStateReadSuccesses] = successes
}

func GetLoadBalancerAgentStateReadSuccesses(labels map[string]interface{}) int {
	if labels == nil {
		return 0
	}
	switch val := labels[LoadBalancerLabelAgentStateReadSuccesses].(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return 0
	}
}

func (d *Database) CreateLoadBalancer(spec api.ApplicationLoadBalancer) (api.ApplicationLoadBalancer, error) {
	mutex, err := d.LockKey("/lock/load-balancer/create")
	if err != nil {
		return api.ApplicationLoadBalancer{}, err
	}
	defer d.UnlockKey(mutex)

	current, err := d.GetLoadBalancers()
	if err != nil && err != ErrNotFound {
		return api.ApplicationLoadBalancer{}, err
	}

	name := strings.TrimSpace(spec.Metadata.Name)
	vnet := strings.TrimSpace(spec.Spec.InternalVirtualNetwork)
	publicIP := strings.TrimSpace(spec.Spec.BindPublicIpAddress)
	for _, lb := range current {
		if strings.TrimSpace(lb.Metadata.Name) == name && strings.TrimSpace(lb.Spec.InternalVirtualNetwork) == vnet {
			return api.ApplicationLoadBalancer{}, fmt.Errorf("load balancer with name %q already exists in internalVirtualNetwork %q", name, vnet)
		}
		if publicIP != "" && strings.TrimSpace(lb.Spec.BindPublicIpAddress) == publicIP {
			return api.ApplicationLoadBalancer{}, fmt.Errorf("bindPublicIpAddress %q is already used by another load balancer", publicIP)
		}
	}

	rec, err := util.DeepCopy(spec)
	if err != nil {
		return api.ApplicationLoadBalancer{}, err
	}

	var key string
	for {
		rec.Metadata.Uuid = util.StringPtr(uuid.New().String())
		id := (*rec.Metadata.Uuid)[:5]
		api.SetLoadBalancerID(&rec, id)
		key = LoadBalancerPrefix + "/" + id

		var existing api.ApplicationLoadBalancer
		_, getErr := d.GetJSON(key, &existing)
		if getErr == ErrNotFound {
			break
		}
		if getErr != nil {
			return api.ApplicationLoadBalancer{}, getErr
		}
	}

	now := time.Now()
	rec.Status = &api.Status{
		StatusCode:          LOAD_BALANCER_PENDING,
		Status:              util.StringPtr(LoadBalancerStatus[LOAD_BALANCER_PENDING]),
		CreationTimeStamp:   util.TimePtr(now),
		LastUpdateTimeStamp: util.TimePtr(now),
	}
	if err := d.PutJSON(key, rec); err != nil {
		return api.ApplicationLoadBalancer{}, err
	}

	return rec, nil
}

func (d *Database) GetLoadBalancers() ([]api.ApplicationLoadBalancer, error) {
	result := make([]api.ApplicationLoadBalancer, 0)
	resp, err := d.GetByPrefix(LoadBalancerPrefix)
	if err == ErrNotFound {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	for _, kv := range resp.Kvs {
		var rec api.ApplicationLoadBalancer
		if err := json.Unmarshal(kv.Value, &rec); err != nil {
			slog.Error("GetLoadBalancers() unmarshal failed", "err", err, "key", string(kv.Key))
			continue
		}
		result = append(result, rec)
	}
	return result, nil
}

func (d *Database) GetLoadBalancerById(id string) (api.ApplicationLoadBalancer, error) {
	key := LoadBalancerPrefix + "/" + id
	var rec api.ApplicationLoadBalancer
	_, err := d.GetJSON(key, &rec)
	if err != nil {
		return api.ApplicationLoadBalancer{}, err
	}
	return rec, nil
}

func (d *Database) UpdateLoadBalancerById(id string, spec api.ApplicationLoadBalancer) error {
	for {
		err := d.updateLoadBalancer(id, spec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

func (d *Database) updateLoadBalancer(id string, spec api.ApplicationLoadBalancer) error {
	lockKey := "/lock/load-balancer/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	key := LoadBalancerPrefix + "/" + id
	var rec api.ApplicationLoadBalancer
	resp, err := d.GetJSON(key, &rec)
	if err != nil {
		return err
	}
	util.PatchStruct(&rec, spec)
	api.SetLoadBalancerID(&rec, id)
	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, &rec)
}

func (d *Database) DeleteLoadBalancerById(id string) error {
	lockKey := "/lock/load-balancer/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)
	return d.DeleteJSON(LoadBalancerPrefix + "/" + id)
}

func (d *Database) SetDeleteTimestampLoadBalancer(id string) error {
	for {
		rec, err := d.GetLoadBalancerById(id)
		if err != nil {
			return err
		}
		if rec.Status == nil {
			rec.Status = &api.Status{}
		}
		rec.Status.StatusCode = LOAD_BALANCER_DELETING
		rec.Status.Status = util.StringPtr(LoadBalancerStatus[LOAD_BALANCER_DELETING])
		rec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
		if rec.Status.DeletionTimeStamp == nil {
			rec.Status.DeletionTimeStamp = util.TimePtr(time.Now())
		}
		rec.Status.Message = nil

		err = d.putLoadBalancerById(rec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

func (d *Database) UpdateLoadBalancerStatusWithMessage(id string, status int, message string) error {
	for {
		rec, err := d.GetLoadBalancerById(id)
		if err != nil {
			return err
		}
		if rec.Status == nil {
			rec.Status = &api.Status{}
		}
		statusChanged := rec.Status.StatusCode != status
		rec.Status.StatusCode = status
		rec.Status.Status = util.StringPtr(LoadBalancerStatus[status])
		rec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
		trimmed := strings.TrimSpace(message)
		if statusChanged {
			rec.Status.Message = nil
		}
		if trimmed != "" {
			rec.Status.Message = util.StringPtr(trimmed)
		}

		err = d.putLoadBalancerById(rec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

func (d *Database) putLoadBalancerById(rec api.ApplicationLoadBalancer) error {
	id := api.LoadBalancerID(rec)
	if strings.TrimSpace(id) == "" {
		return ErrNotFound
	}

	lockKey := "/lock/load-balancer/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	key := LoadBalancerPrefix + "/" + id
	var current api.ApplicationLoadBalancer
	resp, err := d.GetJSON(key, &current)
	if err != nil {
		return err
	}
	api.SetLoadBalancerID(&rec, id)
	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, &rec)
}
