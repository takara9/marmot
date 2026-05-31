package networkfabric

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var ovnMapPairPattern = regexp.MustCompile(`"([^"]+)"\s*=\s*"([^"]*)"`)

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

	if _, err := runOVNNBCTLCommand("--may-exist", "lb-add", lbName); err != nil {
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

	if _, err := runOVNNBCTLCommand("clear", "load_balancer", lbName, "vips"); err != nil {
		return "", fmt.Errorf("failed to clear vips on OVN load balancer %s: %w", lbName, err)
	}
	if err := setLoadBalancerVIPs(lbName, spec.VIPs); err != nil {
		return "", err
	}

	if _, err := runOVNNBCTLCommand("--may-exist", "ls-lb-add", lsName, lbName); err != nil {
		return "", fmt.Errorf("failed to attach OVN load balancer %s to logical switch %s: %w", lbName, lsName, err)
	}

	return lbName, nil
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
