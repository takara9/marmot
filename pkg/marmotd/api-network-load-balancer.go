package marmotd

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

func (s *Server) ApiCreateNetworkLoadBalancer(ctx echo.Context) error {
	var rec api.NetworkLoadBalancer
	if err := ctx.Bind(&rec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "invalid request body"})
	}

	assignNodeNameIfUnset(&rec.Metadata, s.Ma.NodeName)
	if err := normalizeNetworkLoadBalancerResource(&rec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}

	created, err := s.Ma.Db.CreateNetworkLoadBalancer(rec)
	if err != nil {
		if errors.Is(err, db.ErrFound) {
			return ctx.JSON(http.StatusConflict, api.Error{Code: 1, Message: err.Error()})
		}
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusCreated, created)
}

func (s *Server) ApiGetNetworkLoadBalancers(ctx echo.Context) error {
	items, err := s.Ma.Db.GetNetworkLoadBalancers()
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, items)
}

func (s *Server) ApiGetNetworkLoadBalancerById(ctx echo.Context, id string) error {
	rec, err := s.Ma.Db.GetNetworkLoadBalancerById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, rec)
}

func (s *Server) ApiUpdateNetworkLoadBalancerById(ctx echo.Context, id string) error {
	current, err := s.Ma.Db.GetNetworkLoadBalancerById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	var req api.NetworkLoadBalancer
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "invalid request body"})
	}
	if err := normalizeNetworkLoadBalancerSpec(&req.Spec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}
	if changedImmutableNetworkLoadBalancerField(current, req) {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "immutable fields changed: spec.bindPublicIpAddress, spec.internalVirtualNetwork"})
	}

	current.Spec.Listeners = req.Spec.Listeners
	current.Spec.RemoteCIDR = req.Spec.RemoteCIDR
	if err := s.Ma.Db.UpdateNetworkLoadBalancerById(id, current); err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("Accepted the request to update the network load balancer")})
}

func (s *Server) ApiDeleteNetworkLoadBalancerById(ctx echo.Context, id string) error {
	if _, err := s.Ma.Db.GetNetworkLoadBalancerById(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	if err := s.Ma.Db.SetDeleteTimestampNetworkLoadBalancer(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("Accepted the request to delete the network load balancer")})
}

func changedImmutableNetworkLoadBalancerField(current api.NetworkLoadBalancer, req api.NetworkLoadBalancer) bool {
	return strings.TrimSpace(current.Spec.BindPublicIpAddress) != strings.TrimSpace(req.Spec.BindPublicIpAddress) ||
		strings.TrimSpace(current.Spec.InternalVirtualNetwork) != strings.TrimSpace(req.Spec.InternalVirtualNetwork)
}

func normalizeNetworkLoadBalancerResource(rec *api.NetworkLoadBalancer) error {
	if rec == nil {
		return fmt.Errorf("network load balancer is nil")
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
	return normalizeNetworkLoadBalancerSpec(&rec.Spec)
}

func normalizeNetworkLoadBalancerSpec(spec *api.NetworkLoadBalancerSpec) error {
	if spec == nil {
		return fmt.Errorf("spec is required")
	}
	publicIP := strings.TrimSpace(spec.BindPublicIpAddress)
	if publicIP == "" {
		return fmt.Errorf("spec.bindPublicIpAddress is required")
	}
	ip := net.ParseIP(publicIP)
	if ip == nil {
		return fmt.Errorf("spec.bindPublicIpAddress must be a valid IP address")
	}
	if ip.To4() == nil {
		return fmt.Errorf("spec.bindPublicIpAddress must be an IPv4 address in initial release")
	}
	spec.BindPublicIpAddress = publicIP

	spec.InternalVirtualNetwork = strings.TrimSpace(spec.InternalVirtualNetwork)
	if spec.InternalVirtualNetwork == "" {
		return fmt.Errorf("spec.internalVirtualNetwork is required")
	}

	spec.RemoteCIDR = strings.TrimSpace(spec.RemoteCIDR)
	if spec.RemoteCIDR != "" {
		if _, _, err := net.ParseCIDR(spec.RemoteCIDR); err != nil {
			return fmt.Errorf("spec.remoteCIDR must be a valid CIDR")
		}
	}

	if len(spec.Listeners) == 0 {
		return fmt.Errorf("spec.listeners must contain at least one listener")
	}

	seenNames := make(map[string]struct{}, len(spec.Listeners))
	seenVipPorts := make(map[int]struct{}, len(spec.Listeners))
	for index := range spec.Listeners {
		listener := &spec.Listeners[index]
		listener.Name = strings.TrimSpace(listener.Name)
		if listener.Name == "" {
			return fmt.Errorf("spec.listeners[%d].name is required", index)
		}
		if _, exists := seenNames[listener.Name]; exists {
			return fmt.Errorf("spec.listeners[%d].name %q is duplicated", index, listener.Name)
		}
		seenNames[listener.Name] = struct{}{}

		listener.Protocol = strings.ToUpper(strings.TrimSpace(listener.Protocol))
		if listener.Protocol != "TCP" && listener.Protocol != "UDP" {
			return fmt.Errorf("spec.listeners[%d].protocol must be one of TCP/UDP", index)
		}
		if listener.VipPort < 1 || listener.VipPort > 65535 {
			return fmt.Errorf("spec.listeners[%d].vipPort must be between 1 and 65535", index)
		}
		if listener.BackendPort < 1 || listener.BackendPort > 65535 {
			return fmt.Errorf("spec.listeners[%d].backendPort must be between 1 and 65535", index)
		}
		if _, exists := seenVipPorts[listener.VipPort]; exists {
			return fmt.Errorf("spec.listeners[%d].vipPort %d is duplicated", index, listener.VipPort)
		}
		seenVipPorts[listener.VipPort] = struct{}{}
		if len(listener.BackendSelector.MatchLabels) == 0 {
			return fmt.Errorf("spec.listeners[%d].backendSelector.matchLabels is required", index)
		}
		for key, value := range listener.BackendSelector.MatchLabels {
			delete(listener.BackendSelector.MatchLabels, key)
			listener.BackendSelector.MatchLabels[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}

		if listener.SessionPersistence == nil {
			listener.SessionPersistence = &api.NetworkLoadBalancerPersistence{Enabled: false}
		}
	}

	return nil
}
