package networkfabric

import (
	"fmt"
	"hash/fnv"
	"net"
	"regexp"
	"sort"
	"strings"
)

var ovnMapPairPattern = regexp.MustCompile(`"([^"]+)"\s*=\s*"([^"]*)"`)
var ovnDuplicateVIPPortPattern = regexp.MustCompile(`duplicate IPv4 address '[^']+' found on logical switch port '([^']+)'`)

func (o *OVNFabric) EnsureLoadBalancer(spec OVNLoadBalancerSpec) (string, error) {
	if _, err := ovnNBCTLLookPath("ovn-nbctl"); err != nil {
		return "", fmt.Errorf("ovn-nbctl is required for load balancer operations")
	}

	lbName := ovnLoadBalancerName(spec.LoadBalancerID)
	if lbName == "" {
		return "", fmt.Errorf("load balancer id is required")
	}
	lsName := strings.TrimSpace(spec.LogicalSwitchName)
	if lsName == "" {
		return "", fmt.Errorf("logical switch name is required")
	}
	protocol := strings.ToLower(strings.TrimSpace(spec.Protocol))
	if protocol != "tcp" && protocol != "udp" {
		return "", fmt.Errorf("load balancer protocol must be tcp or udp")
	}

	if err := ensureNamedLoadBalancer(lbName); err != nil {
		return "", fmt.Errorf("failed to ensure OVN load balancer %s: %w", lbName, err)
	}

	externalIDs := map[string]string{
		"marmot_loadbalancer_id": strings.TrimSpace(spec.LoadBalancerID),
		"marmot_managed":         "true",
	}
	for k, v := range spec.ExternalIDs {
		tk := strings.TrimSpace(k)
		if tk == "" {
			continue
		}
		externalIDs[tk] = strings.TrimSpace(v)
	}
	keys := sortedMapKeys(externalIDs)
	for _, key := range keys {
		val := externalIDs[key]
		if _, err := runOVNNBCTLCommand("set", "load_balancer", lbName, fmt.Sprintf("external_ids:%s=%s", key, val)); err != nil {
			return "", fmt.Errorf("failed to set external_id %s on OVN load balancer %s: %w", key, lbName, err)
		}
	}

	if _, err := runOVNNBCTLCommand("set", "load_balancer", lbName, "protocol="+protocol); err != nil {
		return "", fmt.Errorf("failed to set protocol %s on OVN load balancer %s: %w", protocol, lbName, err)
	}

	if _, err := runOVNNBCTLCommand("clear", "load_balancer", lbName, "vips"); err != nil {
		return "", fmt.Errorf("failed to clear vips on OVN load balancer %s: %w", lbName, err)
	}
	if err := setLoadBalancerVIPs(lbName, spec.VIPs); err != nil {
		return "", err
	}
	if err := purgeLegacyLoadBalancerVIPPorts(lsName); err != nil {
		return "", err
	}

	if _, err := runOVNNBCTLCommand("--may-exist", "ls-lb-add", lsName, lbName); err != nil {
		return "", fmt.Errorf("failed to attach OVN load balancer %s to logical switch %s: %w", lbName, lsName, err)
	}

	return lbName, nil
}

func ensureNamedLoadBalancer(lbName string) error {
	output, err := runOVNNBCTLCommand("--columns=_uuid", "--format=csv", "--data=bare", "--no-heading", "find", "load_balancer", "name="+lbName)
	if err != nil {
		return err
	}
	if strings.TrimSpace(firstCSVLine(output)) != "" {
		return nil
	}
	if _, err := runOVNNBCTLCommand("create", "load_balancer", "name="+lbName); err != nil {
		return err
	}
	return nil
}

func (o *OVNFabric) DeleteLoadBalancer(loadBalancerID string, logicalSwitchName string) error {
	if _, err := ovnNBCTLLookPath("ovn-nbctl"); err != nil {
		return fmt.Errorf("ovn-nbctl is required for load balancer operations")
	}

	lbName := ovnLoadBalancerName(loadBalancerID)
	if lbName == "" {
		return fmt.Errorf("load balancer id is required")
	}
	lsName := strings.TrimSpace(logicalSwitchName)
	if lsName != "" {
		if _, err := runOVNNBCTLCommand("--if-exists", "ls-lb-del", lsName, lbName); err != nil {
			if !isOVNNoRowError(err) {
				return fmt.Errorf("failed to detach OVN load balancer %s from logical switch %s: %w", lbName, lsName, err)
			}
		}
		if err := purgeLegacyLoadBalancerVIPPorts(lsName); err != nil {
			return err
		}
	}
	if _, err := runOVNNBCTLCommand("--if-exists", "lb-del", lbName); err != nil {
		return fmt.Errorf("failed to delete OVN load balancer %s: %w", lbName, err)
	}
	return nil
}

func (o *OVNFabric) GetLoadBalancerStatus(loadBalancerID string, logicalSwitchName string) (OVNLoadBalancerStatus, error) {
	if _, err := ovnNBCTLLookPath("ovn-nbctl"); err != nil {
		return OVNLoadBalancerStatus{}, fmt.Errorf("ovn-nbctl is required for load balancer operations")
	}

	lbName := ovnLoadBalancerName(loadBalancerID)
	if lbName == "" {
		return OVNLoadBalancerStatus{}, fmt.Errorf("load balancer id is required")
	}
	status := OVNLoadBalancerStatus{OVNLoadBalancerName: lbName}

	uuidOutput, err := runOVNNBCTLCommand("--columns=_uuid", "--format=csv", "--data=bare", "--no-heading", "find", "load_balancer", "name="+lbName)
	if err != nil {
		return status, fmt.Errorf("failed to get OVN load balancer uuid for %s: %w", lbName, err)
	}
	uuid := strings.TrimSpace(firstCSVLine(uuidOutput))
	if uuid == "" {
		return status, nil
	}
	status.Exists = true

	vipsOut, err := runOVNNBCTLCommand("get", "load_balancer", lbName, "vips")
	if err != nil {
		return status, fmt.Errorf("failed to get OVN load balancer vips for %s: %w", lbName, err)
	}
	status.VIPCount = len(parseOVNMap(vipsOut))

	lsName := strings.TrimSpace(logicalSwitchName)
	if lsName == "" {
		return status, nil
	}
	attachedOut, err := runOVNNBCTLCommand("get", "logical_switch", lsName, "load_balancer")
	if err != nil {
		if isOVNNoRowError(err) {
			return status, nil
		}
		return status, fmt.Errorf("failed to get OVN logical switch load balancer refs for %s: %w", lsName, err)
	}
	status.AttachedToLogicalSwitch = strings.Contains(attachedOut, uuid)
	return status, nil
}

func setLoadBalancerVIPs(lbName string, vips map[string]string) error {
	for _, key := range sortedMapKeys(vips) {
		vipKey := strings.TrimSpace(key)
		backendValue := strings.TrimSpace(vips[key])
		if vipKey == "" || backendValue == "" {
			return fmt.Errorf("vips key and value are required")
		}
		if _, err := runOVNNBCTLCommand("set", "load_balancer", lbName, fmt.Sprintf("vips:\"%s\"=\"%s\"", vipKey, backendValue)); err != nil {
			return fmt.Errorf("failed to set vip mapping %s on OVN load balancer %s: %w", vipKey, lbName, err)
		}
	}
	return nil
}

func purgeLegacyLoadBalancerVIPPorts(logicalSwitchName string) error {
	lsName := strings.TrimSpace(logicalSwitchName)
	if lsName == "" {
		return nil
	}
	ports, err := listLogicalSwitchPorts(lsName)
	if err != nil {
		if isOVNNoRowError(err) {
			return nil
		}
		return err
	}
	for _, port := range ports {
		port = strings.TrimSpace(port)
		if !strings.HasPrefix(port, "marmot-lb-vip-") {
			continue
		}
		if _, err := runOVNNBCTLCommand("--if-exists", "lsp-del", port); err != nil {
			return fmt.Errorf("failed to delete legacy vip logical switch port %s: %w", port, err)
		}
	}
	return nil
}

func ensureLoadBalancerVIPPorts(loadBalancerID, logicalSwitchName string, vips map[string]string) error {
	lsName := strings.TrimSpace(logicalSwitchName)
	if lsName == "" {
		return fmt.Errorf("logical switch name is required for vip ports")
	}
	vipIPs, err := collectUniqueVIPIPs(vips)
	if err != nil {
		return err
	}

	existing, err := listLogicalSwitchPorts(lsName)
	if err != nil {
		return err
	}

	// Remove stale marmot LB VIP ports that already claim the same VIP on this LS.
	for _, vipIP := range vipIPs {
		desiredPort := loadBalancerVIPPortName(loadBalancerID, vipIP)
		for _, port := range existing {
			port = strings.TrimSpace(port)
			if !strings.HasPrefix(port, "marmot-lb-vip-") {
				continue
			}
			if port == desiredPort {
				continue
			}
			addressesOut, getErr := runOVNNBCTLCommand("get", "logical_switch_port", port, "addresses")
			if getErr != nil {
				if isOVNNoRowError(getErr) {
					continue
				}
				return fmt.Errorf("failed to inspect vip logical switch port %s: %w", port, getErr)
			}
			if !strings.Contains(addressesOut, vipIP) {
				continue
			}
			if _, delErr := runOVNNBCTLCommand("--if-exists", "lsp-del", port); delErr != nil {
				return fmt.Errorf("failed to delete stale vip logical switch port %s for vip %s: %w", port, vipIP, delErr)
			}
		}
	}

	desiredPorts := map[string]struct{}{}
	for _, vipIP := range vipIPs {
		portName := loadBalancerVIPPortName(loadBalancerID, vipIP)
		desiredPorts[portName] = struct{}{}
		if _, err := runOVNNBCTLCommand("--may-exist", "lsp-add", lsName, portName); err != nil {
			return fmt.Errorf("failed to ensure vip logical switch port %s on %s: %w", portName, lsName, err)
		}
		if _, err := runOVNNBCTLCommand("set", "logical_switch_port", portName, "type=localport"); err != nil {
			return fmt.Errorf("failed to set vip port type on %s: %w", portName, err)
		}
		addresses := fmt.Sprintf("%s %s", loadBalancerVIPPortMAC(loadBalancerID, vipIP), vipIP)
		if _, err := runOVNNBCTLCommand("lsp-set-addresses", portName, addresses); err != nil {
			conflictPort := duplicateVIPConflictPortName(err)
			if conflictPort != "" && conflictPort != portName && strings.HasPrefix(conflictPort, "marmot-lb-vip-") {
				if _, delErr := runOVNNBCTLCommand("--if-exists", "lsp-del", conflictPort); delErr != nil {
					return fmt.Errorf("failed to delete conflicting vip logical switch port %s: %w", conflictPort, delErr)
				}
				if _, retryErr := runOVNNBCTLCommand("lsp-set-addresses", portName, addresses); retryErr != nil {
					return fmt.Errorf("failed to set vip port addresses on %s after deleting conflicting port %s: %w", portName, conflictPort, retryErr)
				}
			} else {
				return fmt.Errorf("failed to set vip port addresses on %s: %w", portName, err)
			}
		}
		if _, err := runOVNNBCTLCommand("set", "logical_switch_port", portName,
			"external_ids:marmot_managed=true",
			"external_ids:marmot_loadbalancer_id="+strings.TrimSpace(loadBalancerID),
			"external_ids:marmot_vip_ip="+vipIP,
		); err != nil {
			return fmt.Errorf("failed to set external_ids on vip port %s: %w", portName, err)
		}
	}
	prefix := loadBalancerVIPPortPrefix(loadBalancerID)
	for _, port := range existing {
		if !strings.HasPrefix(strings.TrimSpace(port), prefix) {
			continue
		}
		if _, ok := desiredPorts[port]; ok {
			continue
		}
		if _, err := runOVNNBCTLCommand("--if-exists", "lsp-del", port); err != nil {
			return fmt.Errorf("failed to delete stale vip logical switch port %s: %w", port, err)
		}
	}

	return nil
}

func deleteLoadBalancerVIPPorts(loadBalancerID, logicalSwitchName string) error {
	lsName := strings.TrimSpace(logicalSwitchName)
	if lsName == "" {
		return nil
	}
	ports, err := listLogicalSwitchPorts(lsName)
	if err != nil {
		if isOVNNoRowError(err) {
			return nil
		}
		return err
	}
	prefix := loadBalancerVIPPortPrefix(loadBalancerID)
	for _, port := range ports {
		if !strings.HasPrefix(strings.TrimSpace(port), prefix) {
			continue
		}
		if _, err := runOVNNBCTLCommand("--if-exists", "lsp-del", port); err != nil {
			return fmt.Errorf("failed to delete vip logical switch port %s: %w", port, err)
		}
	}
	return nil
}

func collectUniqueVIPIPs(vips map[string]string) ([]string, error) {
	unique := map[string]struct{}{}
	for raw := range vips {
		ip, err := vipAddressFromKey(raw)
		if err != nil {
			return nil, err
		}
		unique[ip] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for ip := range unique {
		result = append(result, ip)
	}
	sort.Strings(result)
	return result, nil
}

func vipAddressFromKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", fmt.Errorf("vips key is required")
	}
	host := key
	if idx := strings.LastIndex(key, ":"); idx > 0 {
		host = key[:idx]
	}
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	if net.ParseIP(host) == nil {
		return "", fmt.Errorf("invalid vip address in key %q", raw)
	}
	return host, nil
}

func loadBalancerVIPPortPrefix(loadBalancerID string) string {
	id := strings.TrimSpace(loadBalancerID)
	id = ovnLogicalSwitchSanitizer.ReplaceAllString(id, "-")
	if id == "" {
		id = "unknown"
	}
	return "marmot-lb-vip-" + id + "-"
}

func loadBalancerVIPPortName(loadBalancerID, vipIP string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(loadBalancerID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(vipIP)))
	return fmt.Sprintf("%s%08x", loadBalancerVIPPortPrefix(loadBalancerID), h.Sum32())
}

func loadBalancerVIPPortMAC(loadBalancerID, vipIP string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(loadBalancerID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(vipIP)))
	sum := h.Sum32()
	return fmt.Sprintf("02:00:%02x:%02x:%02x:%02x", byte(sum>>24), byte(sum>>16), byte(sum>>8), byte(sum))
}

func parseOVNMap(raw string) map[string]string {
	result := map[string]string{}
	for _, m := range ovnMapPairPattern.FindAllStringSubmatch(strings.TrimSpace(raw), -1) {
		if len(m) < 3 {
			continue
		}
		result[strings.TrimSpace(m[1])] = strings.TrimSpace(m[2])
	}
	return result
}

func ovnLoadBalancerName(loadBalancerID string) string {
	id := strings.TrimSpace(loadBalancerID)
	if id == "" {
		return ""
	}
	safe := ovnLogicalSwitchSanitizer.ReplaceAllString(id, "-")
	if safe == "" {
		return ""
	}
	return "marmot-lb-" + safe
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstCSVLine(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return strings.Trim(trimmed, `"`)
		}
	}
	return ""
}

func isOVNNoRowError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no row") || strings.Contains(msg, "does not exist")
}

func duplicateVIPConflictPortName(err error) string {
	if err == nil {
		return ""
	}
	matches := ovnDuplicateVIPPortPattern.FindStringSubmatch(err.Error())
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}
