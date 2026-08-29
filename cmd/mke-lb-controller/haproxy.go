package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// renderHAProxyConfig は、VIPが払い出し済み(VIP != "")のLoadBalancer Serviceのみを対象に、
// HAProxyのfrontend/backend設定を生成する。VIP未払い出しのServiceは呼び出し側でスキップされる
// 前提のため、ここでは対象外として扱わない。
func renderHAProxyConfig(nodes []nodeInfo, services []loadBalancerServiceInfo) string {
	var b strings.Builder
	b.WriteString("global\n    log /dev/log local0\n    maxconn 4096\n\n")
	b.WriteString("defaults\n    mode tcp\n    timeout connect 5s\n    timeout client 60s\n    timeout server 60s\n\n")

	sorted := make([]loadBalancerServiceInfo, 0, len(services))
	for _, svc := range services {
		if strings.TrimSpace(svc.VIP) == "" {
			continue
		}
		sorted = append(sorted, svc)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace != sorted[j].Namespace {
			return sorted[i].Namespace < sorted[j].Namespace
		}
		return sorted[i].Name < sorted[j].Name
	})

	for _, svc := range sorted {
		for _, port := range svc.Ports {
			blockName := fmt.Sprintf("%s_%s_%d", svc.Namespace, svc.Name, port.Port)
			fmt.Fprintf(&b, "frontend fe_%s\n", blockName)
			fmt.Fprintf(&b, "    bind %s:%d\n", svc.VIP, port.Port)
			b.WriteString("    mode tcp\n    option tcplog\n")
			fmt.Fprintf(&b, "    default_backend be_%s\n\n", blockName)

			fmt.Fprintf(&b, "backend be_%s\n", blockName)
			b.WriteString("    mode tcp\n    balance roundrobin\n")
			for _, node := range nodes {
				if strings.TrimSpace(node.InternalIP) == "" {
					continue
				}
				fmt.Fprintf(&b, "    server %s %s:%d check\n", node.Name, node.InternalIP, port.NodePort)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// applyHAProxyConfig は、生成した設定内容をhaproxy -cで検証してから反映し、
// systemctl reload haproxy を実行する。反映前の内容と同一の場合は何もしない。
func applyHAProxyConfig(path, content, lastAppliedHash string) (appliedHash string, changed bool, err error) {
	hash := sha256.Sum256([]byte(content))
	newHash := hex.EncodeToString(hash[:])
	if newHash == lastAppliedHash {
		return lastAppliedHash, false, nil
	}

	tmpPath := path + ".candidate"
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return lastAppliedHash, false, fmt.Errorf("failed to write candidate config: %w", err)
	}
	if output, err := exec.Command("haproxy", "-c", "-f", tmpPath).CombinedOutput(); err != nil {
		_ = os.Remove(tmpPath)
		return lastAppliedHash, false, fmt.Errorf("haproxy config validation failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return lastAppliedHash, false, fmt.Errorf("failed to activate config: %w", err)
	}
	if output, err := exec.Command("systemctl", "reload", "haproxy").CombinedOutput(); err != nil {
		return lastAppliedHash, false, fmt.Errorf("haproxy reload failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return newHash, true, nil
}
