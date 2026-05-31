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
	LOAD_BALANCER_PROVISIONING = 1
	LOAD_BALANCER_ACTIVE       = 2
	LOAD_BALANCER_FAILED       = 3
	LOAD_BALANCER_DELETING     = 4

	LoadBalancerLabelManagedBy      = "managedBy"
	LoadBalancerLabelManagedByValue = "load-balancer-controller"
	LoadBalancerLabelAppliedConfig  = "appliedConfigHash"
	LoadBalancerLabelResolvedVIP    = "resolvedVirtualIpAddress"
)

var LoadBalancerStatus = map[int]string{
	LOAD_BALANCER_PENDING:      "PENDING",
	LOAD_BALANCER_PROVISIONING: "PROVISIONING",
	LOAD_BALANCER_ACTIVE:       "ACTIVE",
	LOAD_BALANCER_FAILED:       "FAILED",
	LOAD_BALANCER_DELETING:     "DELETING",
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

func SetLoadBalancerResolvedVIP(labels map[string]interface{}, vip string) {
	if labels == nil {
		return
	}
	labels[LoadBalancerLabelResolvedVIP] = strings.TrimSpace(vip)
}

func GetLoadBalancerResolvedVIP(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[LoadBalancerLabelResolvedVIP].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func (d *Database) CreateLoadBalancer(spec api.LoadBalancer) (api.LoadBalancer, error) {
	mutex, err := d.LockKey("/lock/load-balancer/create")
	if err != nil {
		return api.LoadBalancer{}, err
	}
	defer d.UnlockKey(mutex)

	current, err := d.GetLoadBalancers()
	if err != nil && err != ErrNotFound {
		return api.LoadBalancer{}, err
	}

	name := strings.TrimSpace(spec.Metadata.Name)
	vnet := strings.TrimSpace(spec.Spec.InternalVirtualNetwork)
	for _, item := range current {
		existingName := strings.TrimSpace(item.Metadata.Name)
		existingVnet := strings.TrimSpace(item.Spec.InternalVirtualNetwork)
		if existingName == name && existingVnet == vnet {
			return api.LoadBalancer{}, fmt.Errorf("load balancer with name %q already exists in internalVirtualNetwork %q", name, vnet)
		}
	}

	rec, err := util.DeepCopy(spec)
	if err != nil {
		return api.LoadBalancer{}, err
	}

	if rec.Metadata.Labels == nil {
		rec.Metadata.Labels = &map[string]interface{}{}
	}
	labels := *rec.Metadata.Labels
	labels[LoadBalancerLabelManagedBy] = LoadBalancerLabelManagedByValue
	rec.Metadata.Labels = &labels

	var key string
	for {
		rec.Metadata.Uuid = util.StringPtr(uuid.New().String())
		id := (*rec.Metadata.Uuid)[:5]
		api.SetLoadBalancerID(&rec, id)
		key = LoadBalancerPrefix + "/" + id

		var existing api.LoadBalancer
		_, getErr := d.GetJSON(key, &existing)
		if getErr == ErrNotFound {
			break
		}
		if getErr != nil {
			return api.LoadBalancer{}, getErr
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
		return api.LoadBalancer{}, err
	}

	return rec, nil
}

func (d *Database) GetLoadBalancers() ([]api.LoadBalancer, error) {
	result := make([]api.LoadBalancer, 0)
	resp, err := d.GetByPrefix(LoadBalancerPrefix)
	if err == ErrNotFound {
		return result, nil
	}
	if err != nil {
		return result, err
	}

	for _, kv := range resp.Kvs {
		var rec api.LoadBalancer
		if err := json.Unmarshal(kv.Value, &rec); err != nil {
			slog.Error("GetLoadBalancers() unmarshal failed", "err", err, "key", string(kv.Key))
			continue
		}
		result = append(result, rec)
	}

	return result, nil
}

func (d *Database) GetLoadBalancerById(id string) (api.LoadBalancer, error) {
	key := LoadBalancerPrefix + "/" + id
	var rec api.LoadBalancer
	_, err := d.GetJSON(key, &rec)
	if err != nil {
		return api.LoadBalancer{}, err
	}
	return rec, nil
}

func (d *Database) UpdateLoadBalancerById(id string, spec api.LoadBalancer) error {
	for {
		err := d.updateLoadBalancerById(id, spec)
		if err == ErrUpdateConflict {
			slog.Warn("UpdateLoadBalancerById() retrying due to update conflict", "loadBalancerId", id)
			continue
		}
		return err
	}
}

func (d *Database) updateLoadBalancerById(id string, spec api.LoadBalancer) error {
	lockKey := "/lock/load-balancer/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	key := LoadBalancerPrefix + "/" + id
	var rec api.LoadBalancer
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

	key := LoadBalancerPrefix + "/" + id
	return d.DeleteJSON(key)
}

func (d *Database) SetDeleteTimestampLoadBalancer(id string) error {
	rec, err := d.GetLoadBalancerById(id)
	if err != nil {
		return err
	}

	now := time.Now()
	if rec.Status == nil {
		rec.Status = &api.Status{}
	}
	rec.Status.DeletionTimeStamp = util.TimePtr(now)
	rec.Status.LastUpdateTimeStamp = util.TimePtr(now)
	rec.Status.StatusCode = LOAD_BALANCER_DELETING
	rec.Status.Status = util.StringPtr(LoadBalancerStatus[LOAD_BALANCER_DELETING])

	return d.UpdateLoadBalancerById(id, rec)
}

func (d *Database) UpdateLoadBalancerStatusWithMessage(id string, status int, message string) error {
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

	trimmedMessage := strings.TrimSpace(message)
	if statusChanged {
		rec.Status.Message = util.StringPtr("")
	}
	if trimmedMessage == "" {
		if rec.Status.Message == nil {
			rec.Status.Message = util.StringPtr("")
		}
	} else {
		rec.Status.Message = util.StringPtr(trimmedMessage)
	}

	return d.UpdateLoadBalancerById(id, rec)
}

func (d *Database) GetLoadBalancerByName(name string) ([]api.LoadBalancer, error) {
	all, err := d.GetLoadBalancers()
	if err != nil {
		return nil, err
	}

	result := make([]api.LoadBalancer, 0)
	target := strings.TrimSpace(name)
	for _, item := range all {
		if strings.TrimSpace(item.Metadata.Name) == target {
			result = append(result, item)
		}
	}
	return result, nil
}
