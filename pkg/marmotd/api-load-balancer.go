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

func (s *Server) ApiCreateLoadBalancer(ctx echo.Context) error {
	var rec api.ApplicationLoadBalancer
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
	rec, err := s.Ma.Db.GetLoadBalancerById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, rec)
}

func (s *Server) ApiUpdateLoadBalancerById(ctx echo.Context, id string) error {
	current, err := s.Ma.Db.GetLoadBalancerById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	var req api.ApplicationLoadBalancer
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "invalid request body"})
	}
	if err := normalizeLoadBalancerSpec(&req.Spec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}
	if changedImmutableLoadBalancerField(current, req) {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "immutable fields changed: spec.bindPublicIpAddress, spec.internalVirtualNetwork"})
	}

	current.Spec.Listeners = req.Spec.Listeners
	current.Spec.RemoteCIDR = req.Spec.RemoteCIDR
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

func changedImmutableLoadBalancerField(current api.ApplicationLoadBalancer, req api.ApplicationLoadBalancer) bool {
	return strings.TrimSpace(current.Spec.BindPublicIpAddress) != strings.TrimSpace(req.Spec.BindPublicIpAddress) ||
		strings.TrimSpace(current.Spec.InternalVirtualNetwork) != strings.TrimSpace(req.Spec.InternalVirtualNetwork)
}

func normalizeLoadBalancerResource(rec *api.ApplicationLoadBalancer) error {
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
		if listener.Protocol != "HTTP" && listener.Protocol != "TCP" && listener.Protocol != "UDP" {
			return fmt.Errorf("spec.listeners[%d].protocol must be one of HTTP/TCP/UDP", index)
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

		listener.LoadBalancingAlgorithm = strings.ToLower(strings.TrimSpace(listener.LoadBalancingAlgorithm))
		if listener.LoadBalancingAlgorithm == "" {
			listener.LoadBalancingAlgorithm = "roundrobin"
		}
		if listener.LoadBalancingAlgorithm != "roundrobin" && listener.LoadBalancingAlgorithm != "random" {
			return fmt.Errorf("spec.listeners[%d].loadBalancingAlgorithm must be random or roundrobin", index)
		}

		if listener.HealthCheck != nil {
			listener.HealthCheck.Path = strings.TrimSpace(listener.HealthCheck.Path)
			if listener.HealthCheck.Enabled && listener.Protocol != "HTTP" {
				return fmt.Errorf("spec.listeners[%d].healthCheck can be enabled only for HTTP listeners", index)
			}
		}
		if listener.SessionPersistence != nil {
			listener.SessionPersistence.CookieName = strings.TrimSpace(listener.SessionPersistence.CookieName)
			if listener.SessionPersistence.Enabled && listener.Protocol != "HTTP" {
				return fmt.Errorf("spec.listeners[%d].sessionPersistence can be enabled only for HTTP listeners", index)
			}
		}
	}

	return nil
}