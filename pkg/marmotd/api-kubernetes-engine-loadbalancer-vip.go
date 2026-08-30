package marmotd

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

// kubernetesEngineLoadBalancerVipFQDN は、フェーズ11仕様の内部DNS命名規則
// (サービス名.ネームスペース名.MKEクラスタ名.HVホスト名.labo.local)でFQDNを組み立てる。
func kubernetesEngineLoadBalancerVipFQDN(ke api.KubernetesEngine, namespace, serviceName string) (string, error) {
	if ke.Metadata.NodeName == nil || strings.TrimSpace(*ke.Metadata.NodeName) == "" {
		return "", fmt.Errorf("kubernetes engine %q has no assigned host node", ke.Metadata.Name)
	}
	return fmt.Sprintf("%s.%s.%s.%s.labo.local",
		strings.TrimSpace(serviceName),
		strings.TrimSpace(namespace),
		strings.TrimSpace(ke.Metadata.Name),
		strings.TrimSpace(*ke.Metadata.NodeName),
	), nil
}

// ApiCreateKubernetesEngineLoadBalancerVip は、host-bridge IPAMプールからVIPを1つ払い出し、
// 内部DNSへ登録する。既にDNS登録済みの場合は同じVIPを返す(冪等)。
func (s *Server) ApiCreateKubernetesEngineLoadBalancerVip(ctx echo.Context, id string) error {
	var req api.KubernetesEngineLoadBalancerVipRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "invalid request body"})
	}
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.ServiceName) == "" {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "namespace and serviceName are required"})
	}

	ke, err := s.Ma.Db.GetKubernetesEngineById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	fqdn, err := kubernetesEngineLoadBalancerVipFQDN(ke, req.Namespace, req.ServiceName)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}

	if existingVIP, getErr := s.Ma.Db.GetDnsEntryFQDN(fqdn); getErr == nil && strings.TrimSpace(existingVIP) != "" {
		return ctx.JSON(http.StatusCreated, api.KubernetesEngineLoadBalancerVip{Vip: existingVIP, Fqdn: fqdn})
	}

	vnet, err := s.Ma.Db.GetVirtualNetworkByName("host-bridge")
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: "host-bridge network is not available: " + err.Error()})
	}
	ipnet, err := s.Ma.ensureHostBridgeIPNetwork(&vnet)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	vnetID := api.VirtualNetworkID(vnet)
	vip, _, err := s.Ma.Db.AllocateIP(vnetID, ipnet.Id, fqdn)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if err := s.Ma.Db.PutDnsEntryFQDN(fqdn, vip); err != nil {
		if relErr := s.Ma.Db.ReleaseIP(vnetID, ipnet.Id, vip); relErr != nil {
			slog.Warn("ApiCreateKubernetesEngineLoadBalancerVip: ReleaseIP() failed after PutDnsEntryFQDN() error", "err", relErr)
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusCreated, api.KubernetesEngineLoadBalancerVip{Vip: vip, Fqdn: fqdn})
}

// ApiDeleteKubernetesEngineLoadBalancerVip は、内部DNSエントリーを削除し、払い出し済みVIPを
// IPAMプールへ返却する。未払い出しの場合は何もせず成功として扱う(冪等)。
func (s *Server) ApiDeleteKubernetesEngineLoadBalancerVip(ctx echo.Context, id string, namespace string, serviceName string) error {
	ke, err := s.Ma.Db.GetKubernetesEngineById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	fqdn, err := kubernetesEngineLoadBalancerVipFQDN(ke, namespace, serviceName)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}

	vip, err := s.Ma.Db.GetDnsEntryFQDN(fqdn)
	if err != nil || strings.TrimSpace(vip) == "" {
		return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("VIP is not allocated")})
	}

	if err := s.Ma.Db.DeleteDnsEntryFQDN(fqdn); err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	vnet, err := s.Ma.Db.GetVirtualNetworkByName("host-bridge")
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: "host-bridge network is not available: " + err.Error()})
	}
	if vnet.Spec.IpNetworkId == nil || strings.TrimSpace(*vnet.Spec.IpNetworkId) == "" {
		return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("VIP DNS entry removed")})
	}
	if err := s.Ma.Db.ReleaseIP(api.VirtualNetworkID(vnet), strings.TrimSpace(*vnet.Spec.IpNetworkId), vip); err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("VIP released")})
}
