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
	GATEWAY_PENDING      = 0
	GATEWAY_PROVISIONING = 1
	GATEWAY_CONFIGURING  = 2
	GATEWAY_ACTIVE       = 3
	GATEWAY_FAILED       = 4
	GATEWAY_DELETING     = 5
)

var GatewayStatus = map[int]string{
	0: "PENDING",
	1: "PROVISIONING",
	2: "CONFIGURING",
	3: "ACTIVE",
	4: "FAILED",
	5: "DELETING",
}

// CreateGateway stores a gateway object in etcd with a generated ID and PENDING status.
func (d *Database) CreateGateway(spec api.Gateway) (api.Gateway, error) {
	mutex, err := d.LockKey("/lock/gateway/create")
	if err != nil {
		return api.Gateway{}, err
	}
	defer d.UnlockKey(mutex)

	gateways, err := d.GetGateways()
	if err != nil && err != ErrNotFound {
		slog.Error("CreateGateway() GetGateways() failed", "err", err)
		return api.Gateway{}, err
	}

	name := strings.TrimSpace(spec.Metadata.Name)
	internalVNet := strings.TrimSpace(spec.Spec.InternalVirtualNetwork)
	publicIP := strings.TrimSpace(spec.Spec.BindPublicIpAddress)

	for _, g := range gateways {
		existingName := strings.TrimSpace(g.Metadata.Name)
		existingVNet := strings.TrimSpace(g.Spec.InternalVirtualNetwork)
		existingPublicIP := strings.TrimSpace(g.Spec.BindPublicIpAddress)

		if existingName == name && existingVNet == internalVNet {
			return api.Gateway{}, fmt.Errorf("gateway with name %q already exists in internalVirtualNetwork %q", name, internalVNet)
		}
		if existingPublicIP != "" && publicIP != "" && existingPublicIP == publicIP {
			return api.Gateway{}, fmt.Errorf("bindPublicIpAddress %q is already used by another gateway", publicIP)
		}
	}

	gateway, err := util.DeepCopy(spec)
	if err != nil {
		return api.Gateway{}, err
	}

	var key string
	for {
		gateway.Metadata.Uuid = util.StringPtr(uuid.New().String())
		id := (*gateway.Metadata.Uuid)[:5]
		api.SetGatewayID(&gateway, id)
		key = GatewayPrefix + "/" + id

		var existing api.Gateway
		_, getErr := d.GetJSON(key, &existing)
		if getErr == ErrNotFound {
			break
		}
		if getErr != nil {
			return api.Gateway{}, getErr
		}
	}

	now := time.Now()
	gateway.Status = &api.Status{
		StatusCode:          GATEWAY_PENDING,
		Status:              util.StringPtr(GatewayStatus[GATEWAY_PENDING]),
		CreationTimeStamp:   util.TimePtr(now),
		LastUpdateTimeStamp: util.TimePtr(now),
	}

	if err := d.PutJSON(key, gateway); err != nil {
		slog.Error("CreateGateway() PutJSON failed", "err", err, "key", key)
		return api.Gateway{}, err
	}

	return gateway, nil
}

// GetGateways returns all gateways.
func (d *Database) GetGateways() ([]api.Gateway, error) {
	var gateways []api.Gateway
	resp, err := d.GetByPrefix(GatewayPrefix)
	if err == ErrNotFound {
		return gateways, nil
	}
	if err != nil {
		return gateways, err
	}

	for _, kv := range resp.Kvs {
		var gateway api.Gateway
		if err := json.Unmarshal(kv.Value, &gateway); err != nil {
			slog.Error("GetGateways() unmarshal failed", "err", err, "key", string(kv.Key))
			continue
		}
		gateways = append(gateways, gateway)
	}

	return gateways, nil
}

// GetGatewayById returns one gateway by ID.
func (d *Database) GetGatewayById(id string) (api.Gateway, error) {
	key := GatewayPrefix + "/" + id
	var gateway api.Gateway
	_, err := d.GetJSON(key, &gateway)
	if err != nil {
		return api.Gateway{}, err
	}
	return gateway, nil
}

// UpdateGatewayById updates a gateway using optimistic locking.
func (d *Database) UpdateGatewayById(id string, spec api.Gateway) error {
	for {
		err := d.updateGateway(id, spec)
		if err == ErrUpdateConflict {
			slog.Warn("UpdateGatewayById() retrying due to update conflict", "gatewayId", id)
			continue
		}
		return err
	}
}

func (d *Database) updateGateway(id string, spec api.Gateway) error {
	lockKey := "/lock/gateway/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	key := GatewayPrefix + "/" + id
	var rec api.Gateway
	resp, err := d.GetJSON(key, &rec)
	if err != nil {
		return err
	}

	expected := resp.Kvs[0].ModRevision
	util.PatchStruct(&rec, spec)
	api.SetGatewayID(&rec, id)

	return d.PutJSONCAS(key, expected, &rec)
}

// DeleteGatewayById deletes a gateway record by ID.
func (d *Database) DeleteGatewayById(id string) error {
	lockKey := "/lock/gateway/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	key := GatewayPrefix + "/" + id
	return d.DeleteJSON(key)
}

// SetDeleteTimestampGateway sets deletion timestamp for async controller processing.
func (d *Database) SetDeleteTimestampGateway(id string) error {
	gateway, err := d.GetGatewayById(id)
	if err != nil {
		return err
	}

	now := time.Now()
	if gateway.Status == nil {
		gateway.Status = &api.Status{}
	}
	gateway.Status.DeletionTimeStamp = util.TimePtr(now)
	gateway.Status.LastUpdateTimeStamp = util.TimePtr(now)
	gateway.Status.StatusCode = GATEWAY_DELETING
	gateway.Status.Status = util.StringPtr(GatewayStatus[GATEWAY_DELETING])

	return d.UpdateGatewayById(id, gateway)
}

// UpdateGatewayStatusWithMessage updates status and optional message for gateway objects.
func (d *Database) UpdateGatewayStatusWithMessage(id string, status int, message string) error {
	gateway, err := d.GetGatewayById(id)
	if err != nil {
		return err
	}
	if gateway.Status == nil {
		gateway.Status = &api.Status{}
	}
	gateway.Status.StatusCode = status
	gateway.Status.Status = util.StringPtr(GatewayStatus[status])
	gateway.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	if strings.TrimSpace(message) == "" {
		gateway.Status.Message = nil
	} else {
		gateway.Status.Message = util.StringPtr(strings.TrimSpace(message))
	}

	return d.UpdateGatewayById(id, gateway)
}

// GetGatewayByName returns gateways matching the given name.
func (d *Database) GetGatewayByName(name string) ([]api.Gateway, error) {
	all, err := d.GetGateways()
	if err != nil {
		return nil, err
	}
	result := make([]api.Gateway, 0)
	for _, g := range all {
		if strings.TrimSpace(g.Metadata.Name) == strings.TrimSpace(name) {
			result = append(result, g)
		}
	}
	return result, nil
}

// CountGatewayByPublicIP counts gateways that use the specified bind public IP.
func (d *Database) CountGatewayByPublicIP(publicIP string) (int, error) {
	all, err := d.GetGateways()
	if err != nil {
		return 0, err
	}
	count := 0
	target := strings.TrimSpace(publicIP)
	for _, g := range all {
		if strings.TrimSpace(g.Spec.BindPublicIpAddress) == target {
			count++
		}
	}
	return count, nil
}
