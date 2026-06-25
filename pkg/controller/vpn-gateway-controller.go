package controller

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

const (
	VPN_GATEWAY_CONTROLLER_INTERVAL = 15 * time.Second
)

// StartVpnGatewayController starts controller loop for vpn-gateway resources.
func StartVpnGatewayController(node string, etcdUrl string) (*controller, error) {
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

	ticker := time.NewTicker(VPN_GATEWAY_CONTROLLER_INTERVAL)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.vpnGatewayControllerLoop()
			case <-c.stopChan:
				slog.Debug("VPNゲートウェイコントローラー停止")
				return
			}
		}
	}()

	return &c, nil
}

func (c *controller) vpnGatewayControllerLoop() {
	slog.Debug("VPNゲートウェイコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))

	items, err := c.db.GetVpnGateways()
	if err != nil {
		slog.Error("GetVpnGateways() failed", "err", err)
		return
	}

	for _, item := range items {
		id := api.VpnGatewayID(item)
		if ok, assignedNode, reason := evaluateNodeAssignment(&item.Metadata, c.marmot.NodeName); !ok {
			slog.Debug("別ノード割当のVPNゲートウェイをスキップ", "id", id, "name", item.Metadata.Name, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			continue
		}

		if item.Status == nil || item.Status.StatusCode != db.VPN_GATEWAY_DELETING {
			missingServer, err := c.isVpnGatewayManagedServerMissing(item)
			if err != nil {
				slog.Warn("failed to validate vpn gateway managed server", "id", id, "err", err)
				continue
			}
			if missingServer {
				c.deleteVpnGatewayForMissingServer(item)
				continue
			}
		}

		if item.Status == nil {
			_ = c.db.UpdateVpnGatewayStatusWithMessage(id, db.VPN_GATEWAY_PENDING, "")
			continue
		}

		if item.Status.DeletionTimeStamp != nil && time.Since(*item.Status.DeletionTimeStamp) > c.deletionDelay {
			if item.Status.StatusCode != db.VPN_GATEWAY_DELETING {
				_ = c.db.UpdateVpnGatewayStatusWithMessage(id, db.VPN_GATEWAY_DELETING, "")
				continue
			}
		}

		switch item.Status.StatusCode {
		case db.VPN_GATEWAY_PENDING:
			c.reconcileVpnGatewayPending(item)
		case db.VPN_GATEWAY_PROVISIONING:
			c.reconcileVpnGatewayProvisioning(item)
		case db.VPN_GATEWAY_CONFIGURING:
			c.reconcileVpnGatewayConfiguring(item)
		case db.VPN_GATEWAY_ACTIVE:
			c.reconcileVpnGatewayActive(item)
		case db.VPN_GATEWAY_DELETING:
			c.reconcileVpnGatewayDeleting(item)
		case db.VPN_GATEWAY_FAILED:
			slog.Debug("FAILED 状態のVPNゲートウェイを検出", "id", id)
		default:
			slog.Warn("不明なVPNゲートウェイ状態", "id", id, "statusCode", item.Status.StatusCode)
		}
	}
}

func (c *controller) isVpnGatewayManagedServerMissing(vpnGateway api.VpnGateway) (bool, error) {
	serverID := strings.TrimSpace(vpnGatewayManagedServerID(vpnGateway))
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

func (c *controller) deleteVpnGatewayForMissingServer(vpnGateway api.VpnGateway) {
	vpnGatewayID := api.VpnGatewayID(vpnGateway)
	if err := c.db.DeleteVpnGatewayById(vpnGatewayID); err != nil {
		slog.Warn("DeleteVpnGatewayById() failed while auto-deleting vpn gateway", "id", vpnGatewayID, "err", err)
		return
	}
	slog.Debug("vpn gateway deleted because managed server no longer exists", "id", vpnGatewayID)
}

func (c *controller) reconcileVpnGatewayPending(vpnGateway api.VpnGateway) {
	vpnGatewayID := api.VpnGatewayID(vpnGateway)

	if err := validateGatewayInternalNetwork(c.db, vpnGateway.Spec.InternalVirtualNetwork); err != nil {
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, err.Error())
		return
	}

	serverID, err := c.ensureVpnGatewayServerEntry(vpnGateway)
	if err != nil {
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, err.Error())
		return
	}

	if err := c.ensureVpnGatewayManagedServerLabel(vpnGatewayID, serverID); err != nil {
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, err.Error())
		return
	}

	if err := c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_PROVISIONING, "vpn gateway VM provisioning started"); err != nil {
		slog.Warn("UpdateVpnGatewayStatusWithMessage() failed", "id", vpnGatewayID, "err", err)
	}
}

func (c *controller) reconcileVpnGatewayProvisioning(vpnGateway api.VpnGateway) {
	vpnGatewayID := api.VpnGatewayID(vpnGateway)
	serverID := vpnGatewayManagedServerID(vpnGateway)
	if strings.TrimSpace(serverID) == "" {
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_PENDING, "vpn gateway server reference is missing")
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_PENDING, "vpn gateway server not found, recreating")
			return
		}
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, err.Error())
		return
	}

	if server.Status == nil {
		return
	}

	switch server.Status.StatusCode {
	case db.SERVER_RUNNING:
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_CONFIGURING, "vpn gateway VM running, applying ansible")
	case db.SERVER_ERROR:
		msg := "vpn gateway server entered error state"
		if server.Status.Message != nil && strings.TrimSpace(*server.Status.Message) != "" {
			msg = strings.TrimSpace(*server.Status.Message)
		}
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, msg)
	}
}

func (c *controller) reconcileVpnGatewayConfiguring(vpnGateway api.VpnGateway) {
	vpnGatewayID := api.VpnGatewayID(vpnGateway)
	serverID := vpnGatewayManagedServerID(vpnGateway)
	if strings.TrimSpace(serverID) == "" {
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_PENDING, "vpn gateway server reference is missing")
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_PENDING, "vpn gateway server not found, recreating")
			return
		}
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, err.Error())
		return
	}
	if server.Status == nil || server.Status.StatusCode != db.SERVER_RUNNING {
		return
	}

	internalCIDR, err := c.lookupNetworkCIDRByName(vpnGateway.Spec.InternalVirtualNetwork)
	if err != nil {
		c.handleVpnGatewayConfigFailure(vpnGatewayID, err)
		return
	}
	targetIP, err := c.resolveVpnGatewayTargetAddress(vpnGateway)
	if err != nil {
		c.handleVpnGatewayConfigFailure(vpnGatewayID, err)
		return
	}
	playbookPath := filepath.Join(vpnGatewayPlaybookDir, fmt.Sprintf("vpn-gateway-%s.yaml", vpnGatewayID))
	if err := renderVpnGatewayPlaybook(playbookPath, targetIP, vpnGateway, internalCIDR); err != nil {
		c.handleVpnGatewayConfigFailure(vpnGatewayID, err)
		return
	}
	if err := runVpnGatewayPlaybook(playbookPath, vpnGateway.Spec.BindPublicIpAddress, vpnGatewayPrivateKeyPath); err != nil {
		c.handleVpnGatewayConfigFailure(vpnGatewayID, err)
		return
	}

	configHash := desiredVpnGatewayConfigHash(vpnGateway)
	if err := c.updateVpnGatewayLabels(vpnGatewayID, func(labels map[string]interface{}) {
		db.SetVpnGatewayAnsibleRetries(labels, 0)
		db.SetVpnGatewayAppliedConfigHash(labels, configHash)
	}); err != nil {
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, err.Error())
		return
	}
	_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_ACTIVE, "")
}

func (c *controller) reconcileVpnGatewayActive(vpnGateway api.VpnGateway) {
	vpnGatewayID := api.VpnGatewayID(vpnGateway)
	serverID := vpnGatewayManagedServerID(vpnGateway)
	if strings.TrimSpace(serverID) == "" {
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_PENDING, "vpn gateway server reference is missing")
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_PENDING, "vpn gateway server disappeared")
			return
		}
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, err.Error())
		return
	}
	if server.Status != nil && server.Status.StatusCode == db.SERVER_ERROR {
		msg := "vpn gateway server entered error state"
		if server.Status.Message != nil && strings.TrimSpace(*server.Status.Message) != "" {
			msg = strings.TrimSpace(*server.Status.Message)
		}
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, msg)
		return
	}

	if vpnGatewayAppliedConfigHash(vpnGateway) != desiredVpnGatewayConfigHash(vpnGateway) {
		if err := c.updateVpnGatewayLabels(vpnGatewayID, func(labels map[string]interface{}) {
			db.SetVpnGatewayAnsibleRetries(labels, 0)
		}); err != nil {
			_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, err.Error())
			return
		}
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_CONFIGURING, "vpn gateway configuration drift detected")
	}
}

func (c *controller) reconcileVpnGatewayDeleting(vpnGateway api.VpnGateway) {
	vpnGatewayID := api.VpnGatewayID(vpnGateway)
	serverID := vpnGatewayManagedServerID(vpnGateway)
	if strings.TrimSpace(serverID) == "" {
		if err := c.db.DeleteVpnGatewayById(vpnGatewayID); err != nil {
			slog.Warn("DeleteVpnGatewayById() failed", "id", vpnGatewayID, "err", err)
		}
		return
	}

	server, err := c.db.GetServerById(serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			if err := c.db.DeleteVpnGatewayById(vpnGatewayID); err != nil {
				slog.Warn("DeleteVpnGatewayById() failed", "id", vpnGatewayID, "err", err)
			}
			return
		}
		slog.Warn("GetServerById() failed while deleting vpn gateway", "id", vpnGatewayID, "serverId", serverID, "err", err)
		return
	}

	if server.Status == nil || server.Status.StatusCode != db.SERVER_DELETING {
		if err := c.db.SetDeleteTimestamp(serverID); err != nil {
			slog.Warn("SetDeleteTimestamp() failed", "id", vpnGatewayID, "serverId", serverID, "err", err)
		}
	}
}

func (c *controller) ensureVpnGatewayManagedServerLabel(vpnGatewayID string, serverID string) error {
	vpnGateway, err := c.db.GetVpnGatewayById(vpnGatewayID)
	if err != nil {
		return err
	}
	if vpnGateway.Metadata.Labels == nil {
		labels := map[string]interface{}{}
		vpnGateway.Metadata.Labels = &labels
	}
	labels := *vpnGateway.Metadata.Labels
	if db.GetVpnGatewayManagedServerID(labels) == strings.TrimSpace(serverID) {
		return nil
	}
	db.SetVpnGatewayManagedServerID(labels, serverID)
	vpnGateway.Metadata.Labels = &labels
	return c.db.UpdateVpnGatewayById(vpnGatewayID, vpnGateway)
}

func (c *controller) updateVpnGatewayLabels(vpnGatewayID string, mutate func(labels map[string]interface{})) error {
	vpnGateway, err := c.db.GetVpnGatewayById(vpnGatewayID)
	if err != nil {
		return err
	}
	if vpnGateway.Metadata.Labels == nil {
		labels := map[string]interface{}{}
		vpnGateway.Metadata.Labels = &labels
	}
	labels := *vpnGateway.Metadata.Labels
	mutate(labels)
	vpnGateway.Metadata.Labels = &labels
	return c.db.UpdateVpnGatewayById(vpnGatewayID, vpnGateway)
}

func (c *controller) ensureVpnGatewayServerEntry(vpnGateway api.VpnGateway) (string, error) {
	if serverID := vpnGatewayManagedServerID(vpnGateway); strings.TrimSpace(serverID) != "" {
		if _, err := c.db.GetServerById(serverID); err == nil {
			return serverID, nil
		}
	}

	serverName := vpnGatewayServerName(vpnGateway)
	if existing, err := c.findServerByName(serverName); err == nil {
		return api.ServerID(existing), nil
	}

	serverSpec, err := c.buildVpnGatewayServerSpec(vpnGateway, serverName)
	if err != nil {
		return "", err
	}

	created, err := c.db.MakeServerEntry(serverSpec)
	if err != nil {
		return "", err
	}

	return api.ServerID(created), nil
}

func (c *controller) buildVpnGatewayServerSpec(vpnGateway api.VpnGateway, serverName string) (api.Server, error) {
	publicIP := strings.TrimSpace(vpnGateway.Spec.BindPublicIpAddress)
	if publicIP == "" {
		return api.Server{}, fmt.Errorf("vpn gateway bindPublicIpAddress is empty")
	}
	internalNetwork := strings.TrimSpace(vpnGateway.Spec.InternalVirtualNetwork)
	if internalNetwork == "" {
		return api.Server{}, fmt.Errorf("vpn gateway internalVirtualNetwork is empty")
	}

	nics := make([]api.NetworkInterface, 0, 2)
	publicNIC := api.NetworkInterface{Networkname: "host-bridge", Address: util.StringPtr(publicIP)}
	if maskLen, err := c.lookupNetworkMaskLen("host-bridge"); err == nil && maskLen > 0 {
		publicNIC.Netmasklen = util.IntPtrInt(maskLen)
	}
	if vpnGateway.Spec.Routes != nil && len(*vpnGateway.Spec.Routes) > 0 {
		publicNIC.Routes = vpnGateway.Spec.Routes
	} else if gw, err := c.lookupNetworkGateway("host-bridge"); err == nil && strings.TrimSpace(gw) != "" {
		defaultTo := "default"
		publicNIC.Routes = &[]api.Route{{To: &defaultTo, Via: util.StringPtr(gw)}}
	}
	nics = append(nics, publicNIC)

	internalNIC := api.NetworkInterface{Networkname: internalNetwork}
	if ip, err := c.deriveGatewayInternalInterfaceAddress(internalNetwork); err == nil && strings.TrimSpace(ip) != "" {
		internalNIC.Address = util.StringPtr(ip)
	}
	nics = append(nics, internalNIC)

	labels := map[string]interface{}{
		db.VpnGatewayServerLabelGatewayID: api.VpnGatewayID(vpnGateway),
		db.VpnGatewayServerLabelRole:      db.VpnGatewayServerLabelRoleValue,
		db.VpnGatewayLabelManagedBy:       db.VpnGatewayLabelManagedByValue,
	}

	meta := api.Metadata{Name: serverName, Labels: &labels}
	if vpnGateway.Metadata.NodeName != nil {
		node := strings.TrimSpace(*vpnGateway.Metadata.NodeName)
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

func (c *controller) lookupNetworkCIDRByName(networkName string) (string, error) {
	vnet, err := c.db.GetVirtualNetworkByName(strings.TrimSpace(networkName))
	if err != nil {
		return "", err
	}
	if vnet.Spec.IPNetworkAddress != nil {
		if cidr := strings.TrimSpace(*vnet.Spec.IPNetworkAddress); cidr != "" {
			return cidr, nil
		}
	}

	if vnet.Spec.IpNetworkId != nil && strings.TrimSpace(*vnet.Spec.IpNetworkId) != "" {
		vnetID := strings.TrimSpace(api.VirtualNetworkID(vnet))
		if vnetID == "" {
			return "", fmt.Errorf("virtual network id is empty for %s", networkName)
		}
		ipnet, err := c.db.GetIpNetworkById(vnetID, strings.TrimSpace(*vnet.Spec.IpNetworkId))
		if err != nil {
			return "", err
		}
		if ipnet.AddressMaskLen != nil {
			if cidr := strings.TrimSpace(*ipnet.AddressMaskLen); cidr != "" {
				return cidr, nil
			}
		}
		if ipnet.NetworkAddress != nil && ipnet.Netmasklen != nil {
			network := strings.TrimSpace(*ipnet.NetworkAddress)
			if network != "" && *ipnet.Netmasklen > 0 {
				return fmt.Sprintf("%s/%d", network, *ipnet.Netmasklen), nil
			}
		}
	}

	return "", fmt.Errorf("iPNetworkAddress and ipNetworkId-derived CIDR are empty for %s", networkName)
}

func (c *controller) resolveVpnGatewayTargetAddress(vpnGateway api.VpnGateway) (string, error) {
	serverID := strings.TrimSpace(vpnGatewayManagedServerID(vpnGateway))
	if serverID == "" {
		return "", fmt.Errorf("vpn gateway server reference is missing")
	}
	server, err := c.db.GetServerById(serverID)
	if err != nil {
		return "", err
	}
	if server.Spec.NetworkInterface == nil {
		return "", fmt.Errorf("vpn gateway server has no network interface")
	}
	publicIP := strings.TrimSpace(vpnGateway.Spec.BindPublicIpAddress)
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
	return "", fmt.Errorf("vpn gateway public target address is missing")
}

func (c *controller) handleVpnGatewayConfigFailure(vpnGatewayID string, err error) {
	if err == nil {
		return
	}
	retries, labelErr := c.incrementVpnGatewayConfigRetries(vpnGatewayID)
	if labelErr != nil {
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, labelErr.Error())
		return
	}
	message := fmt.Sprintf("vpn gateway ansible apply failed (%d/%d): %v", retries, vpnGatewayAnsibleMaxRetryCount, err)
	if retries >= vpnGatewayAnsibleMaxRetryCount {
		_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_FAILED, message)
		return
	}
	_ = c.db.UpdateVpnGatewayStatusWithMessage(vpnGatewayID, db.VPN_GATEWAY_CONFIGURING, message)
}

func (c *controller) incrementVpnGatewayConfigRetries(vpnGatewayID string) (int, error) {
	next := 0
	err := c.updateVpnGatewayLabels(vpnGatewayID, func(labels map[string]interface{}) {
		next = db.GetVpnGatewayAnsibleRetries(labels) + 1
		db.SetVpnGatewayAnsibleRetries(labels, next)
	})
	return next, err
}

func vpnGatewayManagedServerID(vpnGateway api.VpnGateway) string {
	if vpnGateway.Metadata.Labels == nil {
		return ""
	}
	return db.GetVpnGatewayManagedServerID(*vpnGateway.Metadata.Labels)
}

func vpnGatewayAppliedConfigHash(vpnGateway api.VpnGateway) string {
	if vpnGateway.Metadata.Labels == nil {
		return ""
	}
	return db.GetVpnGatewayAppliedConfigHash(*vpnGateway.Metadata.Labels)
}

func vpnGatewayServerName(vpnGateway api.VpnGateway) string {
	id := strings.TrimSpace(api.VpnGatewayID(vpnGateway))
	if id == "" {
		id = strings.TrimSpace(vpnGateway.Metadata.Name)
	}
	return fmt.Sprintf("vgw-%s", id)
}
