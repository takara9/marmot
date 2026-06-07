package marmotd

import (
	_ "embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

// ApiCreateGateway creates a gateway record in etcd.
func (s *Server) ApiCreateGateway(ctx echo.Context) error {
	var gateway api.Gateway
	if err := ctx.Bind(&gateway); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "invalid request body"})
	}

	assignNodeNameIfUnset(&gateway.Metadata, s.Ma.NodeName)
	if err := normalizeGatewayResource(&gateway); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}

	created, err := s.Ma.Db.CreateGateway(gateway)
	if err != nil {
		if errors.Is(err, db.ErrFound) {
			return ctx.JSON(http.StatusConflict, api.Error{Code: 1, Message: err.Error()})
		}
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusCreated, created)
}

// ApiGetGateways lists gateways.
func (s *Server) ApiGetGateways(ctx echo.Context) error {
	gateways, err := s.Ma.Db.GetGateways()
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, gateways)
}

// ApiGetGatewayById returns one gateway by ID.
func (s *Server) ApiGetGatewayById(ctx echo.Context, id string) error {
	gateway, err := s.Ma.Db.GetGatewayById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, gateway)
}

// ApiUpdateGatewayById updates only mutable fields of gateway resources.
func (s *Server) ApiUpdateGatewayById(ctx echo.Context, id string) error {
	current, err := s.Ma.Db.GetGatewayById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	var req api.Gateway
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "invalid request body"})
	}

	if err := normalizeGatewaySpec(&req.Spec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}

	if changedImmutableGatewayField(current, req) {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "immutable fields changed: spec.bindPublicIpAddress, spec.internalServerName, spec.internalVirtualNetwork"})
	}

	// 更新可能項目は spec.serverPorts, spec.remoteCIDRs のみ。
	current.Spec.ServerPorts = req.Spec.ServerPorts
	current.Spec.RemoteCIDR = req.Spec.RemoteCIDR
	current.Spec.RemoteCIDRs = req.Spec.RemoteCIDRs
	if err := s.Ma.Db.UpdateGatewayById(id, current); err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("Accepted the request to update the gateway")})
}

// ApiDeleteGatewayById marks gateway as deleting.
func (s *Server) ApiDeleteGatewayById(ctx echo.Context, id string) error {
	if _, err := s.Ma.Db.GetGatewayById(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if err := s.Ma.Db.SetDeleteTimestampGateway(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("Accepted the request to delete the gateway")})
}

// ApiGetGatewayCertById returns the generated OpenVPN client profile as text/plain.
func (s *Server) ApiGetGatewayCertById(ctx echo.Context, id string) error {
	gateway, err := s.Ma.Db.GetGatewayById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	networkName := strings.TrimSpace(gateway.Spec.InternalVirtualNetwork)
	if networkName == "" {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "spec.internalVirtualNetwork is empty"})
	}

	certPath := filepath.Join("/var/lib/marmot/vpn", filepath.Base(networkName)+".ovpn")
	content, err := os.ReadFile(certPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "vpn cert is not found"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.Blob(http.StatusOK, "text/plain; charset=utf-8", content)
}

func changedImmutableGatewayField(current api.Gateway, req api.Gateway) bool {
	return strings.TrimSpace(current.Spec.BindPublicIpAddress) != strings.TrimSpace(req.Spec.BindPublicIpAddress) ||
		strings.TrimSpace(current.Spec.InternalServerName) != strings.TrimSpace(req.Spec.InternalServerName) ||
		strings.TrimSpace(current.Spec.InternalVirtualNetwork) != strings.TrimSpace(req.Spec.InternalVirtualNetwork)
}

func normalizeGatewayResource(gateway *api.Gateway) error {
	if gateway == nil {
		return fmt.Errorf("gateway is nil")
	}
	if strings.TrimSpace(gateway.ApiVersion) == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if strings.TrimSpace(gateway.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(gateway.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	return normalizeGatewaySpec(&gateway.Spec)
}

func normalizeGatewaySpec(spec *api.GatewaySpec) error {
	if spec == nil {
		return fmt.Errorf("spec is required")
	}

	publicIP := strings.TrimSpace(spec.BindPublicIpAddress)
	if publicIP == "" {
		return fmt.Errorf("spec.bindPublicIpAddress is required")
	}
	if ip := net.ParseIP(publicIP); ip == nil {
		return fmt.Errorf("spec.bindPublicIpAddress must be a valid IP address")
	}
	spec.BindPublicIpAddress = publicIP

	internalServerName := strings.TrimSpace(spec.InternalServerName)
	if internalServerName == "" {
		return fmt.Errorf("spec.internalServerName is required")
	}
	spec.InternalServerName = internalServerName

	internalVirtualNetwork := strings.TrimSpace(spec.InternalVirtualNetwork)
	if internalVirtualNetwork == "" {
		return fmt.Errorf("spec.internalVirtualNetwork is required")
	}
	spec.InternalVirtualNetwork = internalVirtualNetwork

	if err := normalizeGatewayRemoteCIDRs(spec); err != nil {
		return err
	}

	ports, err := normalizeGatewayPorts(spec.ServerPorts)
	if err != nil {
		return err
	}
	spec.ServerPorts = ports

	return nil
}

func normalizeGatewayRemoteCIDRs(spec *api.GatewaySpec) error {
	if spec == nil {
		return fmt.Errorf("spec is required")
	}

	raw := make([]string, 0)
	if spec.RemoteCIDRs != nil {
		raw = make([]string, 0, len(*spec.RemoteCIDRs)+1)
		for _, cidr := range *spec.RemoteCIDRs {
			raw = append(raw, strings.TrimSpace(cidr))
		}
	}
	if len(raw) == 0 && spec.RemoteCIDR != nil && strings.TrimSpace(*spec.RemoteCIDR) != "" {
		raw = append(raw, strings.TrimSpace(*spec.RemoteCIDR))
	}

	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(raw))
	for _, cidr := range raw {
		if cidr == "" {
			continue
		}
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("spec.remoteCIDRs contains an invalid CIDR: %s", cidr)
		}
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("spec.remoteCIDRs must contain only IPv4 CIDRs")
		}
		if _, exists := seen[cidr]; exists {
			continue
		}
		seen[cidr] = struct{}{}
		normalized = append(normalized, cidr)
	}

	sort.Strings(normalized)
	normalizedCIDRs := append([]string(nil), normalized...)
	spec.RemoteCIDRs = &normalizedCIDRs
	if len(normalized) > 0 {
		first := normalized[0]
		spec.RemoteCIDR = &first
	} else {
		spec.RemoteCIDR = nil
	}

	return nil
}

func normalizeGatewayPorts(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("spec.serverPorts is required")
	}

	normalized := make([]string, 0, len(raw))
	seen := map[string]struct{}{}

	for _, p := range raw {
		entry := strings.TrimSpace(p)
		if entry == "" {
			return nil, fmt.Errorf("spec.serverPorts contains an empty entry")
		}

		portSpec, err := resolvePortSpec(entry)
		if err != nil {
			return nil, err
		}

		if _, ok := seen[portSpec]; ok {
			continue
		}
		seen[portSpec] = struct{}{}
		normalized = append(normalized, portSpec)
	}

	if len(normalized) == 0 {
		return nil, fmt.Errorf("spec.serverPorts is required")
	}

	return normalized, nil
}

func resolvePortSpec(entry string) (string, error) {
	if strings.Contains(entry, "/") {
		parts := strings.Split(entry, "/")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid port format %q", entry)
		}
		port, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || port < 1 || port > 65535 {
			return "", fmt.Errorf("invalid port number in %q", entry)
		}
		proto := strings.ToLower(strings.TrimSpace(parts[1]))
		if proto != "tcp" && proto != "udp" {
			return "", fmt.Errorf("invalid protocol in %q: must be tcp or udp", entry)
		}
		return fmt.Sprintf("%d/%s", port, proto), nil
	}

	if _, err := strconv.Atoi(entry); err == nil {
		return "", fmt.Errorf("numeric port %q must include protocol suffix like /tcp or /udp", entry)
	}

	// service name lookup uses tcp as default protocol by requirement.
	port, err := net.LookupPort("tcp", strings.ToLower(entry))
	if err != nil {
		return "", fmt.Errorf("failed to resolve service name %q with tcp: %w", entry, err)
	}
	return fmt.Sprintf("%d/tcp", port), nil
}
