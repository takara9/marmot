package controller

import (
	"fmt"
	"sort"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

func (c *controller) resolveSingleBackendIP(serverName string, networkName string) (string, error) {
	trimmedServer := strings.TrimSpace(serverName)
	trimmedNetwork := strings.TrimSpace(networkName)
	if trimmedServer == "" {
		return "", fmt.Errorf("spec.internalServerName is required")
	}
	if trimmedNetwork == "" {
		return "", fmt.Errorf("spec.internalVirtualNetwork is required")
	}

	servers, err := c.db.GetServers()
	if err != nil {
		return "", err
	}

	for _, server := range servers {
		if strings.TrimSpace(server.Metadata.Name) != trimmedServer {
			continue
		}
		if ip, ok := serverAddressOnNetwork(server, trimmedNetwork); ok {
			return ip, nil
		}
	}

	return "", fmt.Errorf("internal server %q on network %q does not have an assigned IP address", trimmedServer, trimmedNetwork)
}

func (c *controller) resolveAutoBackendsOnNetwork(networkName string, labelKey string, labelValue string) ([]string, error) {
	trimmedNetwork := strings.TrimSpace(networkName)
	if trimmedNetwork == "" {
		return nil, fmt.Errorf("spec.internalVirtualNetwork is required")
	}

	servers, err := c.db.GetServers()
	if err != nil {
		return nil, err
	}

	ips := make([]string, 0)
	seen := map[string]struct{}{}

	for _, server := range servers {
		if server.Status == nil || server.Status.StatusCode != db.SERVER_RUNNING {
			continue
		}
		if !serverHasLabel(server, labelKey, labelValue) {
			continue
		}
		ip, ok := serverAddressOnNetwork(server, trimmedNetwork)
		if !ok {
			continue
		}
		if _, exists := seen[ip]; exists {
			continue
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
	}

	sort.Strings(ips)
	return ips, nil
}

func serverAddressOnNetwork(server api.Server, networkName string) (string, bool) {
	if server.Spec.NetworkInterface == nil {
		return "", false
	}
	trimmedNetwork := strings.TrimSpace(networkName)
	for _, nic := range *server.Spec.NetworkInterface {
		if strings.TrimSpace(nic.Networkname) != trimmedNetwork || nic.Address == nil {
			continue
		}
		if ip := strings.TrimSpace(*nic.Address); ip != "" {
			return ip, true
		}
	}
	return "", false
}

func serverHasLabel(server api.Server, key string, value string) bool {
	if server.Metadata.Labels == nil {
		return false
	}
	v, ok := (*server.Metadata.Labels)[key]
	if !ok {
		return false
	}
	want := strings.TrimSpace(value)
	switch typed := v.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), want)
	case bool:
		if typed {
			return strings.EqualFold(want, "true")
		}
		return strings.EqualFold(want, "false")
	default:
		return false
	}
}
