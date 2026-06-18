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

	"go.yaml.in/yaml/v3"
)

// SetupHostBridge checks and configures host bridge for marmot
// Returns true if bridge was successfully set up, false otherwise
func SetupHostBridge() bool {
	slog.Debug("Starting host-bridge setup")

	// Check if bridge br0 already exists
	if bridgeExists("br0") {
		slog.Debug("Bridge br0 already exists, skipping setup")
		return configureVirshBridge()
	}

	// Find primary NIC (exclude loopback and WiFi)
	primaryNIC := findPrimaryNIC()
	if primaryNIC == "" {
		slog.Warn("No suitable physical NIC found for bridge setup")
		return false
	}

	slog.Debug("Found primary NIC", "nic", primaryNIC)

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

	preservedEthernets, err := loadPreservedNetplanEthernets("/etc/netplan", primaryNIC)
	if err != nil {
		slog.Warn("Failed to load existing netplan ethernet configs", "err", err)
		preservedEthernets = map[string]Ethernet{}
	}

	// Backup existing netplan files
	if err := backupNetplanFiles(); err != nil {
		slog.Warn("Failed to backup netplan files", "err", err)
		// Continue even if backup fails
	}

	// Create netplan configuration
	netplanFile := "/etc/netplan/00-host-bridge.yaml"
	if err := createNetplanConfig(netplanFile, primaryNIC, bridgeConfig, preservedEthernets); err != nil {
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

	slog.Debug("Host-bridge setup completed successfully")
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

func loadPreservedNetplanEthernets(netplanDir string, primaryNIC string) (map[string]Ethernet, error) {
	files, err := ioutil.ReadDir(netplanDir)
	if err != nil {
		return nil, err
	}

	preserved := make(map[string]Ethernet)
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		if strings.HasSuffix(name, ".marmot.bak") || strings.HasSuffix(name, ".bak") {
			continue
		}

		path := filepath.Join(netplanDir, name)
		input, err := ioutil.ReadFile(path)
		if err != nil {
			slog.Warn("Failed to read netplan file for preserve", "file", path, "err", err)
			continue
		}

		var config NetplanConfig
		if err := yaml.Unmarshal(input, &config); err != nil {
			slog.Warn("Failed to parse netplan file for preserve", "file", path, "err", err)
			continue
		}

		for ifaceName, ethCfg := range config.Network.Ethernets {
			if ifaceName == primaryNIC {
				continue
			}
			preserved[ifaceName] = ethCfg
		}
	}

	return preserved, nil
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
		if !strings.HasSuffix(file.Name(), ".yaml") && !strings.HasSuffix(file.Name(), ".yml") {
			continue
		}

		originalPath := filepath.Join(netplanDir, file.Name())
		backupPath := filepath.Join(netplanDir, file.Name()+".marmot.bak")

		input, err := ioutil.ReadFile(originalPath)
		if err != nil {
			slog.Warn("Failed to read netplan file for backup", "file", originalPath, "err", err)
			continue
		}

		backedUp := false
		if _, err := os.Stat(backupPath); err == nil {
			slog.Debug("Netplan backup already exists", "backup", backupPath)
			backedUp = true
		} else if os.IsNotExist(err) {
			if err := ioutil.WriteFile(backupPath, input, 0644); err != nil {
				slog.Warn("Failed to backup netplan file", "file", originalPath, "backup", backupPath, "err", err)
				continue
			}
			backedUp = true
		} else {
			slog.Warn("Failed to stat netplan backup file", "backup", backupPath, "err", err)
			continue
		}

		slog.Debug("Backed up netplan file", "original", originalPath, "backup", backupPath)

		if backedUp {
			// Remove original file after successful backup or when backup already exists
			if err := os.Remove(originalPath); err != nil {
				slog.Warn("Failed to remove original netplan file", "file", originalPath, "err", err)
			} else {
				slog.Debug("Removed original netplan file", "file", originalPath)
			}
		}
	}

	return nil
}

// createNetplanConfig creates netplan configuration for bridge
func createNetplanConfig(filePath string, nicName string, config bridgeConfigInfo, preservedEthernets map[string]Ethernet) error {
	netplan := NetplanConfig{
		Network: Network{
			Version:  2,
			Renderer: "networkd",
			Ethernets: map[string]Ethernet{
				nicName: {
					DHCP4: false,
					DHCP6: false,
				},
			},
			Bridges: map[string]Bridge{},
		},
	}

	for ifaceName, ethCfg := range preservedEthernets {
		if ifaceName == nicName {
			continue
		}
		netplan.Network.Ethernets[ifaceName] = ethCfg
	}

	bridge := Bridge{
		Interfaces: []string{nicName},
		DHCP4:      false,
		DHCP6:      false,
		Parameters: BridgeParameters{STP: false},
		Nameservers: Nameserver{
			Addresses: []string{"8.8.8.8"},
		},
	}
	if config.IPAddress != "" {
		bridge.Addresses = []string{config.IPAddress}
	}
	if config.Gateway != "" {
		bridge.Routes = []Route{{To: "default", Via: config.Gateway}}
	}
	if config.Domain != "" {
		bridge.Nameservers.Search = []string{config.Domain}
	}
	netplan.Network.Bridges["br0"] = bridge

	data, err := yaml.Marshal(&netplan)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(filePath, data, 0644); err != nil {
		return err
	}

	slog.Debug("Created netplan config", "file", filePath)
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

	slog.Debug("netplan applied successfully")
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
		slog.Debug("host-bridge network already defined in virsh")
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

	slog.Debug("Defined host-bridge network in virsh")

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

	slog.Debug("Started and autostated host-bridge network in virsh")
	return true
}
