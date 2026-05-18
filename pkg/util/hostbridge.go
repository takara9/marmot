package util

import (
	"fmt"
	"io/ioutil"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SetupHostBridge checks and configures host bridge for marmot
// Returns true if bridge was successfully set up, false otherwise
func SetupHostBridge() bool {
	slog.Info("Starting host-bridge setup")

	// Check if bridge br0 already exists
	if bridgeExists("br0") {
		slog.Info("Bridge br0 already exists, skipping setup")
		return configureVirshBridge()
	}

	// Find primary NIC (exclude loopback and WiFi)
	primaryNIC := findPrimaryNIC()
	if primaryNIC == "" {
		slog.Warn("No suitable physical NIC found for bridge setup")
		return false
	}

	slog.Info("Found primary NIC", "nic", primaryNIC)

	// Get DHCP information from current interface
	bridgeConfig := getBridgeConfig(primaryNIC)
	if bridgeConfig.IPAddress == "" || bridgeConfig.Gateway == "" {
		slog.Warn(
			"Required network information is missing; aborting host-bridge setup without changing netplan",
			"nic", primaryNIC,
			"ip_address", bridgeConfig.IPAddress,
			"gateway", bridgeConfig.Gateway,
		)
		return false
	}

	// Backup existing netplan files
	if err := backupNetplanFiles(); err != nil {
		slog.Warn("Failed to backup netplan files", "err", err)
		// Continue even if backup fails
	}

	// Create netplan configuration
	netplanFile := "/etc/netplan/00-host-bridge.yaml"
	if err := createNetplanConfig(netplanFile, primaryNIC, bridgeConfig); err != nil {
		slog.Error("Failed to create netplan config", "file", netplanFile, "err", err)
		return false
	}

	// Apply netplan configuration
	if err := applyNetplan(); err != nil {
		slog.Error("Failed to apply netplan", "err", err)
		return false
	}

	// Flush IP addresses from the physical NIC to avoid duplicate IP
	if err := flushIPAddr(primaryNIC); err != nil {
		slog.Warn("Failed to flush IP addresses from NIC", "nic", primaryNIC, "err", err)
	}

	// Wait for bridge to be created
	time.Sleep(2 * time.Second)

	// Configure virsh bridge
	if !configureVirshBridge() {
		slog.Warn("Failed to configure virsh bridge, but netplan configuration is applied")
		return true // Bridge might still work without virsh configuration
	}

	slog.Info("Host-bridge setup completed successfully")
	return true
}

// flushIPAddr removes all IP addresses from the given NIC
func flushIPAddr(nic string) error {
	cmd := exec.Command("ip", "addr", "flush", "dev", nic)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("flush IP failed: %v, output: %s", err, string(output))
	}
	return nil
}

// bridgeExists checks if a bridge with given name exists
func bridgeExists(bridgeName string) bool {
	// Check using ip link show
	cmd := exec.Command("ip", "link", "show", bridgeName)
	err := cmd.Run()
	return err == nil
}

// findPrimaryNIC finds the first physical NIC (excluding loopback and WiFi)
func findPrimaryNIC() string {
	// Get all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		slog.Error("Failed to get network interfaces", "err", err)
		return ""
	}

	for _, iface := range interfaces {
		// Skip loopback
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// Skip WiFi interfaces (starting with 'w')
		if strings.HasPrefix(iface.Name, "w") {
			continue
		}

		// Check if interface is up and has broadcast capability
		if iface.Flags&net.FlagBroadcast != 0 {
			return iface.Name
		}
	}

	return ""
}

// bridgeConfigInfo holds bridge configuration details
type bridgeConfigInfo struct {
	IPAddress  string
	Gateway    string
	Nameserver string
	Domain     string
}

// getBridgeConfig extracts DHCP configuration from interface
func getBridgeConfig(nicName string) bridgeConfigInfo {
	config := bridgeConfigInfo{}

	// Get IP address
	cmd := exec.Command("ip", "-o", "addr", "show", nicName)
	output, err := cmd.Output()
	if err == nil {
		// Parse output: 2: enp1s0    inet 192.168.1.100/24 brd 192.168.1.255 scope global dynamic enp1s0
		parts := strings.Fields(string(output))
		for i, part := range parts {
			if part == "inet" && i+1 < len(parts) {
				config.IPAddress = parts[i+1]
				break
			}
		}
	}

	// Get gateway
	cmd = exec.Command("ip", "route", "show")
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "via") && strings.Contains(line, "default") {
				// Parse: default via 192.168.1.1 dev enp1s0 proto dhcp src 192.168.1.100 metric 100
				parts := strings.Fields(line)
				for i, part := range parts {
					if part == "via" && i+1 < len(parts) {
						config.Gateway = parts[i+1]
						break
					}
				}
			}
		}
	}

	// Get DNS from /etc/resolv.conf
	resolveConf, err := ioutil.ReadFile("/etc/resolv.conf")
	if err == nil {
		lines := strings.Split(string(resolveConf), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "nameserver") {
				parts := strings.Fields(line)
				if len(parts) > 1 {
					config.Nameserver = parts[1]
					break
				}
			}
			if strings.HasPrefix(line, "domain ") {
				parts := strings.Fields(line)
				if len(parts) > 1 {
					config.Domain = parts[1]
				}
			}
		}
	}

	slog.Debug("Bridge config extracted", "nic", nicName, "ip", config.IPAddress, "gateway", config.Gateway, "nameserver", config.Nameserver)
	return config
}

// backupNetplanFiles backs up existing netplan configuration files
func backupNetplanFiles() error {
	netplanDir := "/etc/netplan"
	files, err := ioutil.ReadDir(netplanDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if file.Name()[0] == '_' {
			continue // Already backed up
		}
		if !strings.HasSuffix(file.Name(), ".yaml") && !strings.HasSuffix(file.Name(), ".yml") {
			continue
		}

		originalPath := filepath.Join(netplanDir, file.Name())
		backupPath := filepath.Join(netplanDir, "_"+file.Name())

		input, err := ioutil.ReadFile(originalPath)
		if err != nil {
			slog.Warn("Failed to read netplan file for backup", "file", originalPath, "err", err)
			continue
		}

		if err := ioutil.WriteFile(backupPath, input, 0644); err != nil {
			slog.Warn("Failed to backup netplan file", "file", originalPath, "backup", backupPath, "err", err)
			continue
		}

		slog.Info("Backed up netplan file", "original", originalPath, "backup", backupPath)

		// Remove original file after successful backup
		if err := os.Remove(originalPath); err != nil {
			slog.Warn("Failed to remove original netplan file", "file", originalPath, "err", err)
		} else {
			slog.Info("Removed original netplan file", "file", originalPath)
		}
	}

	return nil
}

// createNetplanConfig creates netplan configuration for bridge
func createNetplanConfig(filePath string, nicName string, config bridgeConfigInfo) error {
	yaml := fmt.Sprintf(`network:
  version: 2
  renderer: networkd
  ethernets:
    %s:
      dhcp4: false
      dhcp6: false
  bridges:
    br0:
      interfaces: [%s]
`, nicName, nicName)

	// Add IP address if available
	if config.IPAddress != "" {
		yaml += fmt.Sprintf(`      addresses:
        - %s
`, config.IPAddress)
	}

	// Add routes if gateway available
	if config.Gateway != "" {
		yaml += fmt.Sprintf(`      routes:
        - to: default
          via: %s
`, config.Gateway)
	}

	// Add nameservers with default 8.8.8.8
	yaml += `      nameservers:
`
	yaml += `        addresses:
`
	yaml += `          - 8.8.8.8
`

	if config.Domain != "" {
		yaml += `        search:
`
		yaml += fmt.Sprintf(`          - %s
`, config.Domain)
	}

	// Add bridge parameters
	yaml += `      parameters:
        stp: false
      dhcp4: no
      dhcp6: no
`

	if err := ioutil.WriteFile(filePath, []byte(yaml), 0644); err != nil {
		return err
	}

	slog.Info("Created netplan config", "file", filePath)
	return nil
}

// applyNetplan applies netplan configuration
func applyNetplan() error {
	cmd := exec.Command("netplan", "apply")
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("netplan apply failed", "err", err, "output", string(output))
		return err
	}

	slog.Info("netplan applied successfully")
	return nil
}

// configureVirshBridge configures virsh network for bridge
func configureVirshBridge() bool {
	// Check if virsh is available
	cmd := exec.Command("which", "virsh")
	if err := cmd.Run(); err != nil {
		slog.Warn("virsh command not found, skipping virsh bridge configuration")
		return false
	}

	// Check if host-bridge network already defined
	cmd = exec.Command("virsh", "net-info", "host-bridge")
	if err := cmd.Run(); err == nil {
		slog.Info("host-bridge network already defined in virsh")
		// Try to start it if not running
		startVirshBridge()
		return true
	}

	// Define the host-bridge network
	hostBridgeXML := `<network>
  <name>host-bridge</name>
  <forward mode='bridge'/>
  <bridge name='br0'/>
</network>
`

	// Write temporary XML file for virsh
	tmpFile, err := ioutil.TempFile("", "hostbridge-*.xml")
	if err != nil {
		slog.Error("Failed to create temp file for virsh XML", "err", err)
		return false
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(hostBridgeXML)); err != nil {
		slog.Error("Failed to write virsh XML", "err", err)
		tmpFile.Close()
		return false
	}
	tmpFile.Close()

	// Define network
	cmd = exec.Command("virsh", "net-define", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("Failed to define host-bridge network", "err", err, "output", string(output))
		return false
	}

	slog.Info("Defined host-bridge network in virsh")

	// Start and autostart the network
	if !startVirshBridge() {
		return false
	}

	return true
}

// startVirshBridge starts and autostarts the host-bridge network
func startVirshBridge() bool {
	// Start network
	cmd := exec.Command("virsh", "net-start", "host-bridge")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore error if network is already started
		if !strings.Contains(string(output), "already active") {
			slog.Warn("Failed to start host-bridge network", "err", err, "output", string(output))
		}
	}

	// Set autostart
	cmd = exec.Command("virsh", "net-autostart", "host-bridge")
	output, err = cmd.CombinedOutput()
	if err != nil {
		slog.Warn("Failed to autostart host-bridge network", "err", err, "output", string(output))
		return false
	}

	slog.Info("Started and autostated host-bridge network in virsh")
	return true
}
