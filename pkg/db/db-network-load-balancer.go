package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

const (
	NETWORK_LOAD_BALANCER_PENDING     = 0
	NETWORK_LOAD_BALANCER_ACTIVE      = 1
	NETWORK_LOAD_BALANCER_FAILED      = 2
	NETWORK_LOAD_BALANCER_DELETING    = 3
	NETWORK_LOAD_BALANCER_CONFIGURING = 4

	NetworkLoadBalancerLabelAppliedConfig = "appliedConfigHash"
	NetworkLoadBalancerLabelApplyRetries  = "applyRetries"
	NetworkLoadBalancerLabelApplyFailedAt = "applyFailedAt"
)

var NetworkLoadBalancerStatus = map[int]string{
	0: "PENDING",
	1: "ACTIVE",
	2: "FAILED",
	3: "DELETING",
	4: "CONFIGURING",
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

func SetNetworkLoadBalancerApplyRetries(labels map[string]interface{}, retries int) {
	if labels == nil {
		return
	}
	if retries < 0 {
		retries = 0
	}
	labels[NetworkLoadBalancerLabelApplyRetries] = retries
}

func GetNetworkLoadBalancerApplyRetries(labels map[string]interface{}) int {
	if labels == nil {
		return 0
	}
	switch val := labels[NetworkLoadBalancerLabelApplyRetries].(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return 0
	}
}

func SetNetworkLoadBalancerApplyFailedAt(labels map[string]interface{}, at time.Time) {
	if labels == nil {
		return
	}
	if at.IsZero() {
		delete(labels, NetworkLoadBalancerLabelApplyFailedAt)
		return
	}
	labels[NetworkLoadBalancerLabelApplyFailedAt] = at.UTC().Format(time.RFC3339Nano)
}

func GetNetworkLoadBalancerApplyFailedAt(labels map[string]interface{}) time.Time {
	if labels == nil {
		return time.Time{}
	}
	raw, ok := labels[NetworkLoadBalancerLabelApplyFailedAt].(string)
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

func (d *Database) CreateNetworkLoadBalancer(spec api.NetworkLoadBalancer) (api.NetworkLoadBalancer, error) {
	if err := normalizeNetworkLoadBalancerRecord(&spec); err != nil {
		return api.NetworkLoadBalancer{}, err
	}

	current, err := d.GetNetworkLoadBalancers()
	if err != nil {
		return api.NetworkLoadBalancer{}, err
	}
	name := strings.TrimSpace(spec.Metadata.Name)
	vnet := strings.TrimSpace(spec.Spec.InternalVirtualNetwork)
	publicIP := strings.TrimSpace(spec.Spec.BindPublicIpAddress)
	for _, existing := range current {
		if strings.TrimSpace(existing.Metadata.Name) == name && strings.TrimSpace(existing.Spec.InternalVirtualNetwork) == vnet {
			return api.NetworkLoadBalancer{}, fmt.Errorf("network load balancer with name %q already exists in internalVirtualNetwork %q", name, vnet)
		}
		if strings.TrimSpace(existing.Spec.BindPublicIpAddress) == publicIP {
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
		rec.Status.StatusCode = status
		rec.Status.Status = util.StringPtr(NetworkLoadBalancerStatus[status])
		rec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
		if message == "" {
			rec.Status.Message = nil
		} else {
			rec.Status.Message = util.StringPtr(message)
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
		return fmt.Errorf("network load balancer id is empty")
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

func normalizeNetworkLoadBalancerRecord(rec *api.NetworkLoadBalancer) error {
	if rec == nil {
		return fmt.Errorf("network load balancer is nil")
	}
	rec.ApiVersion = strings.TrimSpace(rec.ApiVersion)
	rec.Kind = strings.TrimSpace(rec.Kind)
	rec.Metadata.Name = strings.TrimSpace(rec.Metadata.Name)
	if rec.ApiVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if rec.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if rec.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	return nil
}
