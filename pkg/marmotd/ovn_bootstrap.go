package marmotd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const ovnBootstrapCommandTimeout = 5 * time.Second

var ovnNBCTLBootstrapLookPath = exec.LookPath
var ovnSBCTLBootstrapLookPath = exec.LookPath

// EnsureOVNRuntimeBootstrap は marmotd 起動時に OVS external_ids と OVN central listener を整合させる。
func EnsureOVNRuntimeBootstrap(ma *Marmot, cfg *MarmotdConfig) error {
	if ma == nil {
		return nil
	}
	if cfg == nil {
		cfg = CurrentConfig()
	}
	if _, err := exec.LookPath("ovs-vsctl"); err != nil {
		slog.Warn("ovs-vsctl is not available; skip OVN runtime bootstrap", "err", err)
		return nil
	}

	changed := false

	systemID, _ := getOVSExternalID("system-id")
	if strings.TrimSpace(systemID) == "" {
		hostname := strings.TrimSpace(ma.NodeName)
		if hostname == "" {
			hostname = "marmot"
		}
		if err := setOVSExternalID("system-id", hostname); err != nil {
			return err
		}
		changed = true
	}

	if updated, err := reconcileOVSExternalID("ovn-encap-type", "geneve"); err != nil {
		return err
	} else if updated {
		changed = true
	}

	if ip, ok := resolveOVNEncapIP(ma, cfg); ok {
		if updated, err := reconcileOVSExternalID("ovn-encap-ip", ip); err != nil {
			return err
		} else if updated {
			changed = true
		}
	} else {
		slog.Warn("failed to resolve ovn-encap-ip automatically; keep existing value")
	}

	if remoteURL, ok := resolveOVNRemoteEndpoint(ma, cfg); ok {
		if updated, err := reconcileOVSExternalID("ovn-remote", remoteURL); err != nil {
			return err
		} else if updated {
			changed = true
		}
		if updated, err := ensureOVNCentralListener(ma, cfg, remoteURL); err != nil {
			return err
		} else if updated {
			changed = true
		}
	} else {
		slog.Warn("failed to resolve ovn-remote automatically; keep existing value")
	}

	if !changed {
		return nil
	}

	slog.Info("OVN runtime bootstrap updated OVS external_ids; restarting ovn-controller")
	restartCmd := exec.Command("systemctl", "restart", "ovn-controller")
	if output, err := restartCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restart ovn-controller: %w (output=%s)", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func resolveOVNRemoteEndpoint(ma *Marmot, cfg *MarmotdConfig) (string, bool) {
	if ma == nil || ma.Db == nil {
		return remoteEndpointFromLocalIP(ma, cfg)
	}

	statuses, err := ma.Db.GetAllHostStatus()
	if err == nil {
		for _, st := range statuses {
			if st.NodeName == nil || strings.TrimSpace(*st.NodeName) == "" {
				continue
			}
			if !IsSchedulerLeader(strings.TrimSpace(*st.NodeName), statuses) {
				continue
			}
			if st.IpAddress == nil {
				break
			}
			ip := strings.TrimSpace(*st.IpAddress)
			if ip == "" {
				break
			}
			return "tcp:" + ip + ":6642", true
		}
	}

	return remoteEndpointFromLocalIP(ma, cfg)
}

func remoteEndpointFromLocalIP(ma *Marmot, cfg *MarmotdConfig) (string, bool) {
	ip, ok := resolveOVNCentralIP(ma, cfg)
	if !ok || strings.TrimSpace(ip) == "" {
		return "", false
	}
	return "tcp:" + ip + ":6642", true
}

func resolveOVNEncapIP(ma *Marmot, cfg *MarmotdConfig) (string, bool) {
	return resolveOVNCentralIP(ma, cfg)
}

func resolveOVNCentralIP(ma *Marmot, cfg *MarmotdConfig) (string, bool) {
	if cfg != nil {
		ifName := strings.TrimSpace(cfg.DefaultUnderlayInterface)
		if ifName != "" {
			if ip, ok := firstIPv4ByInterfaceName(ifName); ok {
				return ip, true
			}
		}
	}

	if ma != nil {
		status, err := ma.CollectHostStatus()
		if err == nil && status.IpAddress != nil {
			ip := strings.TrimSpace(*status.IpAddress)
			if ip != "" {
				return ip, true
			}
		}
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", false
	}

	sort.Slice(ifaces, func(i, j int) bool {
		return ifaces[i].Index < ifaces[j].Index
	})

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil || ip4.IsLoopback() {
				continue
			}
			return ip4.String(), true
		}
	}

	return "", false
}

func ensureOVNCentralListener(ma *Marmot, cfg *MarmotdConfig, remoteURL string) (bool, error) {
	if _, err := ovnNBCTLBootstrapLookPath("ovn-nbctl"); err != nil {
		return false, nil
	}
	if _, err := ovnSBCTLBootstrapLookPath("ovn-sbctl"); err != nil {
		return false, nil
	}

	selfRemote, ok := remoteEndpointFromLocalIP(ma, cfg)
	if !ok || !strings.EqualFold(strings.TrimSpace(remoteURL), strings.TrimSpace(selfRemote)) {
		return false, nil
	}

	selfIP, ok := resolveOVNCentralIP(ma, cfg)
	if !ok || strings.TrimSpace(selfIP) == "" {
		return false, nil
	}

	changed := false
	if updated, err := ensureOVNDBConnection("ovn-sbctl", "ptcp:6642:"+selfIP); err != nil {
		return false, err
	} else if updated {
		changed = true
	}
	if updated, err := ensureOVNDBConnection("ovn-nbctl", "ptcp:6641:"+selfIP); err != nil {
		return false, err
	} else if updated {
		changed = true
	}

	return changed, nil
}

func ensureOVNDBConnection(command string, target string) (bool, error) {
	current, err := runOVNDBControl(command, "get-connection")
	if err == nil && strings.Contains(current, target) {
		return false, nil
	}
	if _, err := runOVNDBControl(command, "set-connection", target); err != nil {
		return false, err
	}
	slog.Info("OVN bootstrap set DB listener", "command", command, "target", target)
	return true, nil
}

func runOVNDBControl(command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ovnBootstrapCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w (output=%s)", command, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func reconcileOVSExternalID(key, desired string) (bool, error) {
	desired = strings.TrimSpace(desired)
	if desired == "" {
		return false, nil
	}
	current, _ := getOVSExternalID(key)
	if strings.TrimSpace(current) == desired {
		return false, nil
	}
	if err := setOVSExternalID(key, desired); err != nil {
		return false, err
	}
	return true, nil
}

func firstIPv4ByInterfaceName(interfaceName string) (string, bool) {
	ifName := strings.TrimSpace(interfaceName)
	if ifName == "" {
		return "", false
	}

	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return "", false
	}
	if iface.Flags&net.FlagUp == 0 {
		return "", false
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", false
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil {
			continue
		}
		ip4 := ip.To4()
		if ip4 == nil || ip4.IsLoopback() {
			continue
		}
		return ip4.String(), true
	}

	return "", false
}

func getOVSExternalID(key string) (string, bool) {
	k := strings.TrimSpace(key)
	if k == "" {
		return "", false
	}

	ctx, cancel := context.WithTimeout(context.Background(), ovnBootstrapCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ovs-vsctl", "get", "open_vswitch", ".", "external_ids:"+k)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}

	val := strings.TrimSpace(string(output))
	val = strings.Trim(val, "\"")
	if val == "" || val == "[]" || val == "{}" {
		return "", false
	}

	return val, true
}

func setOVSExternalID(key, value string) error {
	k := strings.TrimSpace(key)
	v := strings.TrimSpace(value)
	if k == "" || v == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), ovnBootstrapCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ovs-vsctl", "set", "open_vswitch", ".", "external_ids:"+k+"="+v)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set OVS external_id %s: %w (output=%s)", k, err, strings.TrimSpace(string(output)))
	}

	slog.Info("OVN bootstrap set OVS external_id", "key", k, "value", v)
	return nil
}
