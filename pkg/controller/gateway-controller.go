package controller

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

const (
	GATEWAY_CONTROLLER_INTERVAL = 15 * time.Second
)

var gatewayReservedInternalNetworks = map[string]struct{}{
	"default":     {},
	"host-bridge": {},
	"ovs-network": {},
}

// StartGatewayController starts controller loop for gateway resources.
func StartGatewayController(node string, etcdUrl string) (*controller, error) {
	var c controller
	var err error

	c.deletionDelay = 15 * time.Second
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	ticker := time.NewTicker(GATEWAY_CONTROLLER_INTERVAL)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.gatewayControllerLoop()
			case <-c.stopChan:
				slog.Info("ゲートウェイコントローラー停止")
				return
			}
		}
	}()

	return &c, nil
}

func (c *controller) gatewayControllerLoop() {
	slog.Debug("ゲートウェイコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))

	gateways, err := c.db.GetGateways()
	if err != nil {
		slog.Error("GetGateways() failed", "err", err)
		return
	}

	for _, gateway := range gateways {
		gatewayID := api.GatewayID(gateway)
		if ok, assignedNode, reason := evaluateNodeAssignment(&gateway.Metadata, c.marmot.NodeName); !ok {
			slog.Debug("別ノード割当のゲートウェイをスキップ", "gatewayId", gatewayID, "gatewayName", gateway.Metadata.Name, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			continue
		}

		if gateway.Status == nil {
			if err := c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_PENDING, ""); err != nil {
				slog.Warn("UpdateGatewayStatusWithMessage() failed", "gatewayId", gatewayID, "err", err)
			}
			continue
		}

		if gateway.Status.DeletionTimeStamp != nil && time.Since(*gateway.Status.DeletionTimeStamp) > c.deletionDelay {
			if gateway.Status.StatusCode != db.GATEWAY_DELETING {
				if err := c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_DELETING, ""); err != nil {
					slog.Warn("UpdateGatewayStatusWithMessage() failed", "gatewayId", gatewayID, "err", err)
				}
			}
		}

		switch gateway.Status.StatusCode {
		case db.GATEWAY_PENDING:
			c.reconcileGatewayPending(gateway)
		case db.GATEWAY_PROVISIONING:
			c.reconcileGatewayProvisioning(gateway)
		case db.GATEWAY_ACTIVE:
			c.reconcileGatewayActive(gateway)
		case db.GATEWAY_DELETING:
			c.reconcileGatewayDeleting(gateway)
		case db.GATEWAY_FAILED:
			slog.Debug("FAILED 状態のゲートウェイを検出", "gatewayId", gatewayID)
		default:
			slog.Warn("不明なゲートウェイ状態", "gatewayId", gatewayID, "statusCode", gateway.Status.StatusCode)
		}
	}
}

func (c *controller) reconcileGatewayPending(gateway api.Gateway) {
	gatewayID := api.GatewayID(gateway)

	if err := validateGatewayInternalNetwork(c.db, gateway.Spec.InternalVirtualNetwork); err != nil {
		_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_FAILED, err.Error())
		return
	}

	serverID, err := c.ensureGatewayServerEntry(gateway)
	if err != nil {
		_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_FAILED, err.Error())
		return
	}

	if err := c.ensureGatewayManagedServerLabel(gatewayID, serverID); err != nil {
		_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_FAILED, err.Error())
		return
	}

	if err := c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_PROVISIONING, "gateway VM provisioning started"); err != nil {
		slog.Warn("UpdateGatewayStatusWithMessage() failed", "gatewayId", gatewayID, "err", err)
	}
}

func (c *controller) reconcileGatewayProvisioning(gateway api.Gateway) {
	gatewayID := api.GatewayID(gateway)
	serverID := gatewayManagedServerID(gateway)
	if strings.TrimSpace(serverID) == "" {
		_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_PENDING, "gateway server reference is missing")
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_PENDING, "gateway server not found, recreating")
			return
		}
		_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_FAILED, err.Error())
		return
	}

	if server.Status == nil {
		return
	}

	switch server.Status.StatusCode {
	case db.SERVER_RUNNING:
		_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_ACTIVE, "")
	case db.SERVER_ERROR:
		msg := "gateway server entered error state"
		if server.Status.Message != nil && strings.TrimSpace(*server.Status.Message) != "" {
			msg = strings.TrimSpace(*server.Status.Message)
		}
		_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_FAILED, msg)
	}
}

func (c *controller) reconcileGatewayActive(gateway api.Gateway) {
	gatewayID := api.GatewayID(gateway)
	serverID := gatewayManagedServerID(gateway)
	if strings.TrimSpace(serverID) == "" {
		_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_PENDING, "gateway server reference is missing")
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_PENDING, "gateway server disappeared")
			return
		}
		_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_FAILED, err.Error())
		return
	}
	if server.Status != nil && server.Status.StatusCode == db.SERVER_ERROR {
		msg := "gateway server entered error state"
		if server.Status.Message != nil && strings.TrimSpace(*server.Status.Message) != "" {
			msg = strings.TrimSpace(*server.Status.Message)
		}
		_ = c.db.UpdateGatewayStatusWithMessage(gatewayID, db.GATEWAY_FAILED, msg)
	}
}

func (c *controller) reconcileGatewayDeleting(gateway api.Gateway) {
	gatewayID := api.GatewayID(gateway)
	serverID := gatewayManagedServerID(gateway)
	if strings.TrimSpace(serverID) == "" {
		if err := c.db.DeleteGatewayById(gatewayID); err != nil {
			slog.Warn("DeleteGatewayById() failed", "gatewayId", gatewayID, "err", err)
		}
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			if err := c.db.DeleteGatewayById(gatewayID); err != nil {
				slog.Warn("DeleteGatewayById() failed", "gatewayId", gatewayID, "err", err)
			}
			return
		}
		slog.Warn("GetServerById() failed while deleting gateway", "gatewayId", gatewayID, "serverId", serverID, "err", err)
		return
	}

	if server.Status == nil || server.Status.StatusCode != db.SERVER_DELETING {
		if err := c.db.SetDeleteTimestamp(serverID); err != nil {
			slog.Warn("SetDeleteTimestamp() failed", "gatewayId", gatewayID, "serverId", serverID, "err", err)
		}
	}
}

func (c *controller) ensureGatewayManagedServerLabel(gatewayID string, serverID string) error {
	gateway, err := c.db.GetGatewayById(gatewayID)
	if err != nil {
		return err
	}
	if gateway.Metadata.Labels == nil {
		labels := map[string]interface{}{}
		gateway.Metadata.Labels = &labels
	}
	labels := *gateway.Metadata.Labels
	if db.GetGatewayManagedServerID(labels) == strings.TrimSpace(serverID) {
		return nil
	}
	db.SetGatewayManagedServerID(labels, serverID)
	gateway.Metadata.Labels = &labels
	return c.db.UpdateGatewayById(gatewayID, gateway)
}

func (c *controller) ensureGatewayServerEntry(gateway api.Gateway) (string, error) {
	if serverID := gatewayManagedServerID(gateway); strings.TrimSpace(serverID) != "" {
		if _, err := c.db.GetServerById(serverID); err == nil {
			return serverID, nil
		}
	}

	serverName := gatewayServerName(gateway)
	if existing, err := c.findServerByName(serverName); err == nil {
		return api.ServerID(existing), nil
	}

	serverSpec, err := c.buildGatewayServerSpec(gateway, serverName)
	if err != nil {
		return "", err
	}

	created, err := c.db.MakeServerEntry(serverSpec)
	if err != nil {
		return "", err
	}

	return api.ServerID(created), nil
}

func (c *controller) buildGatewayServerSpec(gateway api.Gateway, serverName string) (api.Server, error) {
	publicIP := strings.TrimSpace(gateway.Spec.BindPublicIpAddress)
	if publicIP == "" {
		return api.Server{}, fmt.Errorf("gateway bindPublicIpAddress is empty")
	}
	internalNetwork := strings.TrimSpace(gateway.Spec.InternalVirtualNetwork)
	if internalNetwork == "" {
		return api.Server{}, fmt.Errorf("gateway internalVirtualNetwork is empty")
	}

	nics := make([]api.NetworkInterface, 0, 2)
	publicNIC := api.NetworkInterface{Networkname: "host-bridge", Address: util.StringPtr(publicIP)}
	if maskLen, err := c.lookupNetworkMaskLen("host-bridge"); err == nil && maskLen > 0 {
		publicNIC.Netmasklen = util.IntPtrInt(maskLen)
	}
	nics = append(nics, publicNIC)
	nics = append(nics, api.NetworkInterface{Networkname: internalNetwork})

	labels := map[string]interface{}{
		db.GatewayServerLabelGatewayID: api.GatewayID(gateway),
		db.GatewayServerLabelRole:      db.GatewayServerLabelRoleValue,
		db.GatewayLabelManagedBy:       db.GatewayLabelManagedByValue,
	}

	meta := api.Metadata{Name: serverName, Labels: &labels}
	if gateway.Metadata.NodeName != nil {
		node := strings.TrimSpace(*gateway.Metadata.NodeName)
		if node != "" {
			meta.NodeName = util.StringPtr(node)
		}
	}

	spec := api.ServerSpec{
		Cpu:              util.IntPtrInt(1),
		Memory:           util.IntPtrInt(1024),
		OsVariant:        util.StringPtr("ubuntu24.04"),
		NetworkInterface: &nics,
	}

	if key := readGatewayPublicKeyIfExists(); key != "" {
		spec.Auth = &api.Auth{PublicKey: util.StringPtr(key), User: util.StringPtr("root")}
	}

	return api.Server{ApiVersion: "v1", Kind: "Server", Metadata: meta, Spec: spec}, nil
}

func (c *controller) lookupNetworkMaskLen(networkName string) (int, error) {
	vnet, err := c.db.GetVirtualNetworkByName(networkName)
	if err != nil {
		return 0, err
	}
	if vnet.Spec.IpNetworkId == nil || strings.TrimSpace(*vnet.Spec.IpNetworkId) == "" {
		return 0, fmt.Errorf("ipNetworkId is empty for %s", networkName)
	}
	ipnet, err := c.db.GetIpNetworkById(api.VirtualNetworkID(vnet), *vnet.Spec.IpNetworkId)
	if err != nil {
		return 0, err
	}
	if ipnet.Netmasklen == nil {
		return 0, fmt.Errorf("netmasklen is empty for %s", networkName)
	}
	return *ipnet.Netmasklen, nil
}

func (c *controller) findServerByName(name string) (api.Server, error) {
	servers, err := c.db.GetServers()
	if err != nil {
		return api.Server{}, err
	}
	for _, s := range servers {
		if strings.TrimSpace(s.Metadata.Name) == strings.TrimSpace(name) {
			return s, nil
		}
	}
	return api.Server{}, db.ErrNotFound
}

func validateGatewayInternalNetwork(database *db.Database, networkName string) error {
	trimmed := strings.TrimSpace(networkName)
	if trimmed == "" {
		return fmt.Errorf("spec.internalVirtualNetwork is required")
	}
	if _, reserved := gatewayReservedInternalNetworks[trimmed]; reserved {
		return fmt.Errorf("spec.internalVirtualNetwork %q is reserved", trimmed)
	}
	if _, err := database.GetVirtualNetworkByName(trimmed); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fmt.Errorf("internalVirtualNetwork %q is not found", trimmed)
		}
		return err
	}
	return nil
}

func gatewayManagedServerID(gateway api.Gateway) string {
	if gateway.Metadata.Labels == nil {
		return ""
	}
	return db.GetGatewayManagedServerID(*gateway.Metadata.Labels)
}

func gatewayServerName(gateway api.Gateway) string {
	id := strings.TrimSpace(api.GatewayID(gateway))
	if id == "" {
		id = strings.TrimSpace(gateway.Metadata.Name)
	}
	return fmt.Sprintf("igw-%s", id)
}

func readGatewayPublicKeyIfExists() string {
	data, err := os.ReadFile("/etc/marmot/keys/public.key")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
