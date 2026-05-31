package marmotd

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

func (s *Server) ApiCreateLoadBalancer(ctx echo.Context) error {
	var rec api.LoadBalancer
	if err := ctx.Bind(&rec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "invalid request body"})
	}
	assignNodeNameIfUnset(&rec.Metadata, s.Ma.NodeName)
	if err := normalizeLoadBalancerResource(&rec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}
	created, err := s.Ma.Db.CreateLoadBalancer(rec)
	if err != nil {
		if errors.Is(err, db.ErrFound) {
			return ctx.JSON(http.StatusConflict, api.Error{Code: 1, Message: err.Error()})
		}
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusCreated, created)
}

func (s *Server) ApiGetLoadBalancers(ctx echo.Context) error {
	items, err := s.Ma.Db.GetLoadBalancers()
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, items)
}

func (s *Server) ApiGetLoadBalancerById(ctx echo.Context, id string) error {
	item, err := s.Ma.Db.GetLoadBalancerById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, item)
}

func (s *Server) ApiUpdateLoadBalancerById(ctx echo.Context, id string) error {
	current, err := s.Ma.Db.GetLoadBalancerById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	var req api.LoadBalancer
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "invalid request body"})
	}
	if err := normalizeLoadBalancerSpec(&req.Spec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}
	if changedImmutableLoadBalancerField(current, req) {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "immutable fields changed: spec.bindPublicIpAddress, spec.internalVirtualNetwork, spec.virtualIpAddress"})
	}

	current.Spec.BackendMode = req.Spec.BackendMode
	current.Spec.InternalServers = req.Spec.InternalServers
	current.Spec.ServerPorts = req.Spec.ServerPorts
	if err := s.Ma.Db.UpdateLoadBalancerById(id, current); err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("Accepted the request to update the load balancer")})
}

func (s *Server) ApiDeleteLoadBalancerById(ctx echo.Context, id string) error {
	if _, err := s.Ma.Db.GetLoadBalancerById(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	if err := s.Ma.Db.SetDeleteTimestampLoadBalancer(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("Accepted the request to delete the load balancer")})
}

func changedImmutableLoadBalancerField(current api.LoadBalancer, req api.LoadBalancer) bool {
	currentBind := ""
	if current.Spec.BindPublicIpAddress != nil {
		currentBind = strings.TrimSpace(*current.Spec.BindPublicIpAddress)
	}
	reqBind := ""
	if req.Spec.BindPublicIpAddress != nil {
		reqBind = strings.TrimSpace(*req.Spec.BindPublicIpAddress)
	}
	currentVIP := ""
	if current.Spec.VirtualIpAddress != nil {
		currentVIP = strings.TrimSpace(*current.Spec.VirtualIpAddress)
	}
	reqVIP := ""
	if req.Spec.VirtualIpAddress != nil {
		reqVIP = strings.TrimSpace(*req.Spec.VirtualIpAddress)
	}
	return currentBind != reqBind ||
		strings.TrimSpace(current.Spec.InternalVirtualNetwork) != strings.TrimSpace(req.Spec.InternalVirtualNetwork) ||
		currentVIP != reqVIP
}

func normalizeLoadBalancerResource(rec *api.LoadBalancer) error {
	if rec == nil {
		return fmt.Errorf("load balancer is nil")
	}
	if strings.TrimSpace(rec.ApiVersion) == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if strings.TrimSpace(rec.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(rec.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	return normalizeLoadBalancerSpec(&rec.Spec)
}

func normalizeLoadBalancerSpec(spec *api.LoadBalancerSpec) error {
	if spec == nil {
		return fmt.Errorf("spec is required")
	}
	if spec.BindPublicIpAddress != nil && strings.TrimSpace(*spec.BindPublicIpAddress) != "" {
		return fmt.Errorf("spec.bindPublicIpAddress is out of scope for this release")
	}

	vnet := strings.TrimSpace(spec.InternalVirtualNetwork)
	if vnet == "" {
		return fmt.Errorf("spec.internalVirtualNetwork is required")
	}
	spec.InternalVirtualNetwork = vnet

	backendMode := "auto"
	if spec.BackendMode != nil && strings.TrimSpace(*spec.BackendMode) != "" {
		backendMode = strings.ToLower(strings.TrimSpace(*spec.BackendMode))
	}
	if backendMode != "auto" && backendMode != "manual" {
		return fmt.Errorf("spec.backendMode must be manual or auto")
	}
	spec.BackendMode = util.StringPtr(backendMode)

	ports, err := NormalizeServerPorts(spec.ServerPorts)
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		return fmt.Errorf("spec.serverPorts is required")
	}
	spec.ServerPorts = ports

	if spec.VirtualIpAddress != nil && strings.TrimSpace(*spec.VirtualIpAddress) != "" {
		vip := strings.TrimSpace(*spec.VirtualIpAddress)
		if ip := net.ParseIP(vip); ip == nil || ip.To4() == nil {
			return fmt.Errorf("spec.virtualIpAddress must be a valid IPv4 address")
		}
		spec.VirtualIpAddress = util.StringPtr(vip)
	}

	seen := map[string]struct{}{}
	normalizedServers := make([]string, 0, len(spec.InternalServers))
	for _, serverName := range spec.InternalServers {
		trimmed := strings.TrimSpace(serverName)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalizedServers = append(normalizedServers, trimmed)
	}
	sort.Strings(normalizedServers)
	spec.InternalServers = normalizedServers

	if backendMode == "manual" && len(spec.InternalServers) == 0 {
		return fmt.Errorf("spec.internalServers is required when backendMode=manual")
	}
	if backendMode == "auto" && len(spec.InternalServers) > 0 {
		return fmt.Errorf("spec.internalServers is forbidden when backendMode=auto")
	}

	return nil
}