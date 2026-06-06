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
	VPN_GATEWAY_PENDING      = 0
	VPN_GATEWAY_ACTIVE       = 1
	VPN_GATEWAY_FAILED       = 2
	VPN_GATEWAY_DELETING     = 3
	VPN_GATEWAY_PROVISIONING = 4
	VPN_GATEWAY_CONFIGURING  = 5

	VpnGatewayLabelManagedServerID = "vpnGatewayServerId"
	VpnGatewayLabelManagedBy       = "managedBy"
	VpnGatewayLabelManagedByValue  = "vpn-gateway-controller"
	VpnGatewayServerLabelGatewayID = "vpnGatewayId"
	VpnGatewayServerLabelRole      = "role"
	VpnGatewayServerLabelRoleValue = "vpn-gateway"
	VpnGatewayLabelAnsibleRetries  = "ansibleRetries"
	VpnGatewayLabelAppliedConfig   = "appliedConfigHash"
)

var VpnGatewayStatus = map[int]string{
	0: "PENDING",
	1: "ACTIVE",
	2: "FAILED",
	3: "DELETING",
	4: "PROVISIONING",
	5: "CONFIGURING",
}

func SetVpnGatewayManagedServerID(labels map[string]interface{}, serverID string) {
	if labels == nil {
		return
	}
	labels[VpnGatewayLabelManagedServerID] = strings.TrimSpace(serverID)
	labels[VpnGatewayLabelManagedBy] = VpnGatewayLabelManagedByValue
}

func GetVpnGatewayManagedServerID(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[VpnGatewayLabelManagedServerID].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func SetVpnGatewayAnsibleRetries(labels map[string]interface{}, retries int) {
	if labels == nil {
		return
	}
	labels[VpnGatewayLabelAnsibleRetries] = retries
}

func GetVpnGatewayAnsibleRetries(labels map[string]interface{}) int {
	if labels == nil {
		return 0
	}
	switch val := labels[VpnGatewayLabelAnsibleRetries].(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return 0
	}
}

func SetVpnGatewayAppliedConfigHash(labels map[string]interface{}, hash string) {
	if labels == nil {
		return
	}
	labels[VpnGatewayLabelAppliedConfig] = strings.TrimSpace(hash)
}

func GetVpnGatewayAppliedConfigHash(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[VpnGatewayLabelAppliedConfig].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func (d *Database) CreateVpnGateway(spec api.VpnGateway) (api.VpnGateway, error) {
	mutex, err := d.LockKey("/lock/vpn-gateway/create")
	if err != nil {
		return api.VpnGateway{}, err
	}
	defer d.UnlockKey(mutex)

	current, err := d.GetVpnGateways()
	if err != nil && err != ErrNotFound {
		return api.VpnGateway{}, err
	}

	name := strings.TrimSpace(spec.Metadata.Name)
	vnet := strings.TrimSpace(spec.Spec.InternalVirtualNetwork)
	pub := strings.TrimSpace(spec.Spec.BindPublicIpAddress)
	for _, g := range current {
		if strings.TrimSpace(g.Spec.InternalVirtualNetwork) == vnet {
			return api.VpnGateway{}, fmt.Errorf("vpn gateway already exists in internalVirtualNetwork %q", vnet)
		}
		if strings.TrimSpace(g.Metadata.Name) == name && strings.TrimSpace(g.Spec.InternalVirtualNetwork) == vnet {
			return api.VpnGateway{}, fmt.Errorf("vpn gateway with name %q already exists in internalVirtualNetwork %q", name, vnet)
		}
		if pub != "" && strings.TrimSpace(g.Spec.BindPublicIpAddress) == pub {
			return api.VpnGateway{}, fmt.Errorf("bindPublicIpAddress %q is already used by another vpn gateway", pub)
		}
	}

	vpnGateway, err := util.DeepCopy(spec)
	if err != nil {
		return api.VpnGateway{}, err
	}

	var key string
	for {
		vpnGateway.Metadata.Uuid = util.StringPtr(uuid.New().String())
		id := (*vpnGateway.Metadata.Uuid)[:5]
		api.SetVpnGatewayID(&vpnGateway, id)
		key = VpnGatewayPrefix + "/" + id

		var existing api.VpnGateway
		_, getErr := d.GetJSON(key, &existing)
		if getErr == ErrNotFound {
			break
		}
		if getErr != nil {
			return api.VpnGateway{}, getErr
		}
	}

	now := time.Now()
	vpnGateway.Status = &api.Status{
		StatusCode:          VPN_GATEWAY_PENDING,
		Status:              util.StringPtr(VpnGatewayStatus[VPN_GATEWAY_PENDING]),
		CreationTimeStamp:   util.TimePtr(now),
		LastUpdateTimeStamp: util.TimePtr(now),
	}
	if err := d.PutJSON(key, vpnGateway); err != nil {
		return api.VpnGateway{}, err
	}
	return vpnGateway, nil
}

func (d *Database) GetVpnGateways() ([]api.VpnGateway, error) {
	result := make([]api.VpnGateway, 0)
	resp, err := d.GetByPrefix(VpnGatewayPrefix)
	if err == ErrNotFound {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	for _, kv := range resp.Kvs {
		var rec api.VpnGateway
		if err := json.Unmarshal(kv.Value, &rec); err != nil {
			slog.Error("GetVpnGateways() unmarshal failed", "err", err)
			continue
		}
		result = append(result, rec)
	}
	return result, nil
}

func (d *Database) GetVpnGatewayById(id string) (api.VpnGateway, error) {
	key := VpnGatewayPrefix + "/" + id
	var rec api.VpnGateway
	_, err := d.GetJSON(key, &rec)
	if err != nil {
		return api.VpnGateway{}, err
	}
	return rec, nil
}

func (d *Database) UpdateVpnGatewayById(id string, spec api.VpnGateway) error {
	for {
		err := d.updateVpnGateway(id, spec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

func (d *Database) updateVpnGateway(id string, spec api.VpnGateway) error {
	lockKey := "/lock/vpn-gateway/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	key := VpnGatewayPrefix + "/" + id
	var rec api.VpnGateway
	resp, err := d.GetJSON(key, &rec)
	if err != nil {
		return err
	}
	util.PatchStruct(&rec, spec)
	api.SetVpnGatewayID(&rec, id)
	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, &rec)
}

func (d *Database) DeleteVpnGatewayById(id string) error {
	lockKey := "/lock/vpn-gateway/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)
	return d.DeleteJSON(VpnGatewayPrefix + "/" + id)
}

func (d *Database) SetDeleteTimestampVpnGateway(id string) error {
	return d.UpdateVpnGatewayStatusWithMessage(id, VPN_GATEWAY_DELETING, "")
}

func (d *Database) UpdateVpnGatewayStatusWithMessage(id string, status int, message string) error {
	for {
		rec, err := d.GetVpnGatewayById(id)
		if err != nil {
			return err
		}
		if rec.Status == nil {
			rec.Status = &api.Status{}
		}
		statusChanged := rec.Status.StatusCode != status
		rec.Status.StatusCode = status
		rec.Status.Status = util.StringPtr(VpnGatewayStatus[status])
		rec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
		trimmed := strings.TrimSpace(message)
		if statusChanged {
			rec.Status.Message = nil
		}
		if trimmed != "" {
			rec.Status.Message = util.StringPtr(trimmed)
		}

		err = d.putVpnGatewayById(rec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

func (d *Database) putVpnGatewayById(rec api.VpnGateway) error {
	id := api.VpnGatewayID(rec)
	if strings.TrimSpace(id) == "" {
		return ErrNotFound
	}

	lockKey := "/lock/vpn-gateway/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	key := VpnGatewayPrefix + "/" + id
	var current api.VpnGateway
	resp, err := d.GetJSON(key, &current)
	if err != nil {
		return err
	}
	api.SetVpnGatewayID(&rec, id)
	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, &rec)
}
