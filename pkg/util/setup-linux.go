//go:build linux
// +build linux

package util

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

func SetupLinux(spec api.Server) error {
	if spec.Spec.BootVolume == nil {
		return fmt.Errorf("BootVolume is nil")
	}

	// ブートボリュームをマウント
	mountPoint, nbdDev, err := MountVolume(*spec.Spec.BootVolume)
	if err != nil {
		slog.Error("MountVolume failed", "error", err)
		return err
	}
	defer func() {
		_ = UnMountVolume(*spec.Spec.BootVolume, mountPoint, nbdDev)
	}()

	return setupLinuxMountedVolume(spec, mountPoint)
}

func SetupAlpineLinux(spec api.Server) error {
	if spec.Spec.BootVolume == nil {
		return fmt.Errorf("BootVolume is nil")
	}

	mountPoint, nbdDev, err := MountVolume(*spec.Spec.BootVolume)
	if err != nil {
		slog.Error("MountVolume failed", "error", err)
		return err
	}
	defer func() {
		_ = UnMountVolume(*spec.Spec.BootVolume, mountPoint, nbdDev)
	}()

	if err := setupMountedIdentity(spec, mountPoint); err != nil {
		return err
	}

	// mgmtネットワークが常に強制付与されるため、通常ここでnilになることは無い(issue #696)。
	// defaultネットワークへの自動フォールバックは廃止したため、念のため空スライスにする。
	if spec.Spec.NetworkInterface == nil {
		spec.Spec.NetworkInterface = &[]api.NetworkInterface{}
	}

	if err := CreateAlpineInterfaces(*spec.Spec.NetworkInterface, mountPoint); err != nil {
		slog.Error("CreateAlpineInterfaces failed", "error", err)
		return err
	}

	return nil
}

// SetupRockyLinux は Rocky Linux のブートボリュームを初期化する。
// ネットワーク設定は NetworkManager の keyfile 形式で書き込む(Rocky 9 は
// NetworkManager が既定のネットワークマネージャのため、Ubuntu の netplan、
// Alpine の /etc/network/interfaces に相当する)。
func SetupRockyLinux(spec api.Server) error {
	if spec.Spec.BootVolume == nil {
		return fmt.Errorf("BootVolume is nil")
	}

	mountPoint, nbdDev, err := MountVolume(*spec.Spec.BootVolume)
	if err != nil {
		slog.Error("MountVolume failed", "error", err)
		return err
	}
	defer func() {
		_ = UnMountVolume(*spec.Spec.BootVolume, mountPoint, nbdDev)
	}()

	if err := setupMountedIdentity(spec, mountPoint); err != nil {
		return err
	}

	// mgmtネットワークが常に強制付与されるため、通常ここでnilになることは無い(issue #696)。
	// defaultネットワークへの自動フォールバックは廃止したため、念のため空スライスにする。
	if spec.Spec.NetworkInterface == nil {
		spec.Spec.NetworkInterface = &[]api.NetworkInterface{}
	}

	if err := CreateNetworkManagerKeyfiles(*spec.Spec.NetworkInterface, mountPoint); err != nil {
		slog.Error("CreateNetworkManagerKeyfiles failed", "error", err)
		return err
	}

	return nil
}

// SetupAlmaLinux は AlmaLinux のブートボリュームを初期化する。
// ネットワーク設定は Rocky Linux と同様に NetworkManager の keyfile 形式で書き込む
// (AlmaLinux 9 の GenericCloud イメージも NetworkManager が既定のネットワークマネージャ
// のため)。
func SetupAlmaLinux(spec api.Server) error {
	if spec.Spec.BootVolume == nil {
		return fmt.Errorf("BootVolume is nil")
	}

	mountPoint, nbdDev, err := MountVolume(*spec.Spec.BootVolume)
	if err != nil {
		slog.Error("MountVolume failed", "error", err)
		return err
	}
	defer func() {
		_ = UnMountVolume(*spec.Spec.BootVolume, mountPoint, nbdDev)
	}()

	if err := setupMountedIdentity(spec, mountPoint); err != nil {
		return err
	}

	// mgmtネットワークが常に強制付与されるため、通常ここでnilになることは無い(issue #696)。
	// defaultネットワークへの自動フォールバックは廃止したため、念のため空スライスにする。
	if spec.Spec.NetworkInterface == nil {
		spec.Spec.NetworkInterface = &[]api.NetworkInterface{}
	}

	if err := CreateNetworkManagerKeyfiles(*spec.Spec.NetworkInterface, mountPoint); err != nil {
		slog.Error("CreateNetworkManagerKeyfiles failed", "error", err)
		return err
	}

	return nil
}

// SetupDebian11 は Debian 11(bullseye)のブートボリュームを初期化する。
// Debian 11 の GenericCloud イメージには netplan が含まれておらず ifupdown で
// ネットワークを管理するため、Debian 12/13(netplan、util.SetupLinux 経由)とは別に
// ifupdown 形式(CreateIfupdownInterfaces)でネットワーク設定を書き込む(issue #622)。
func SetupDebian11(spec api.Server) error {
	if spec.Spec.BootVolume == nil {
		return fmt.Errorf("BootVolume is nil")
	}

	mountPoint, nbdDev, err := MountVolume(*spec.Spec.BootVolume)
	if err != nil {
		slog.Error("MountVolume failed", "error", err)
		return err
	}
	defer func() {
		_ = UnMountVolume(*spec.Spec.BootVolume, mountPoint, nbdDev)
	}()

	if err := setupMountedIdentity(spec, mountPoint); err != nil {
		return err
	}

	// mgmtネットワークが常に強制付与されるため、通常ここでnilになることは無い(issue #696)。
	// defaultネットワークへの自動フォールバックは廃止したため、念のため空スライスにする。
	if spec.Spec.NetworkInterface == nil {
		spec.Spec.NetworkInterface = &[]api.NetworkInterface{}
	}

	if err := CreateIfupdownInterfaces(*spec.Spec.NetworkInterface, mountPoint); err != nil {
		slog.Error("CreateIfupdownInterfaces failed", "error", err)
		return err
	}

	return nil
}

func setupLinuxMountedVolume(spec api.Server, mountPoint string) error {
	if err := setupMountedIdentity(spec, mountPoint); err != nil {
		return err
	}

	// ネットワーク設定。mgmtネットワークが常に強制付与されるため、通常ここでnilになることは
	// 無い(issue #696)。defaultネットワークへの自動フォールバックは廃止したため、念のため
	// 空スライスにする。
	if spec.Spec.NetworkInterface == nil {
		spec.Spec.NetworkInterface = &[]api.NetworkInterface{}
	}

	if err := CreateNetplanInterfaces(*spec.Spec.NetworkInterface, mountPoint); err != nil {
		slog.Error("CreateNetplanInterfaces failed", "error", err)
		return err
	}

	// apt_cacher_ng_enabled が有効な場合のみ、mgmtネットワーク経由でapt-cacher-ngを
	// 使うようAPTプロキシを設定する(issue #696)。無効な環境(apt-cacher-ng未設定)で
	// 強制すると、パッケージ取得自体が全て失敗するため既定は無効。
	if IsAptCacherNGEnabled() {
		if err := writeAptCacherNGProxyConfig(mountPoint); err != nil {
			slog.Error("writeAptCacherNGProxyConfig failed", "error", err)
			return err
		}
	}

	return nil
}

// managementNetworkAptCacherAddress / managementNetworkAptCacherPort は
// marmotd.ManagementNetworkHostAddress ("10.245.0.1")上で稼働するapt-cacher-ngの
// 固定アドレス・ポート(issue #696)。pkg/marmotd が pkg/util に依存するため、
// 循環参照を避けるためここに複製している。値を変更する場合は両方を同期すること。
const managementNetworkAptCacherAddress = "10.245.0.1"
const managementNetworkAptCacherPort = 3142

// writeAptCacherNGProxyConfig は、ゲストOSがMarmotホスト上のapt-cacher-ngを
// パッケージ取得プロキシとして使うようAPT設定を書き込む(issue #696)。
// apt-cacher-ngはデフォルトでHTTPS CONNECTトンネルを拒否するため、HTTPSリポジトリ
// (例: download.docker.com)はプロキシを経由させず直接アクセスさせる(issue #710)。
func writeAptCacherNGProxyConfig(mountPoint string) error {
	aptConfDir := filepath.Join(mountPoint, "etc/apt/apt.conf.d")
	if err := os.MkdirAll(aptConfDir, 0755); err != nil {
		return fmt.Errorf("failed to create apt.conf.d directory: %w", err)
	}

	proxyFile := filepath.Join(aptConfDir, "95marmot-apt-cacher-ng")
	content := fmt.Sprintf("Acquire::http::Proxy \"http://%s:%d\";\nAcquire::https::Proxy \"DIRECT\";\n", managementNetworkAptCacherAddress, managementNetworkAptCacherPort)
	if err := os.WriteFile(proxyFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write apt-cacher-ng proxy config: %w", err)
	}
	return nil
}

func setupMountedIdentity(spec api.Server, mountPoint string) error {
	hostnameFile := filepath.Join(mountPoint, "etc/hostname")
	hostname := hostnameForServer(spec)
	slog.Debug("Setting hostname", "file", hostnameFile, "hostname", hostname)

	if err := os.WriteFile(hostnameFile, []byte(hostname), 0644); err != nil {
		slog.Error("WriteFile hostname failed", "error", err)
		return err
	}

	machineID := machineIDForServer(spec)
	machineIDFile := filepath.Join(mountPoint, "etc/machine-id")
	if err := os.WriteFile(machineIDFile, []byte(machineID), 0644); err != nil {
		slog.Error("WriteFile /etc/machine-id failed", "error", err)
		return err
	}

	hostidFile := filepath.Join(mountPoint, "etc/hostid")
	if err := os.WriteFile(hostidFile, hostIDBytes(machineID), 0644); err != nil {
		slog.Error("WriteFile /etc/hostid failed", "error", err)
		return err
	}

	return nil
}

func CreateAlpineInterfaces(requestConfig []api.NetworkInterface, mountPoint string) error {
	nicNames := []string{"eth0", "eth1", "eth2", "eth3", "eth4", "eth5"}
	var builder strings.Builder
	builder.WriteString("auto lo\n")
	builder.WriteString("iface lo inet loopback\n\n")

	resolvLines := make([]string, 0)
	searchDomains := make([]string, 0)

	if len(requestConfig) == 0 {
		builder.WriteString("auto eth0\n")
		builder.WriteString("iface eth0 inet dhcp\n")
	} else {
		for idx, nic := range requestConfig {
			ifaceName := nicNames[idx]
			builder.WriteString(fmt.Sprintf("auto %s\n", ifaceName))

			if nic.Address != nil && nic.Netmasklen != nil {
				builder.WriteString(fmt.Sprintf("iface %s inet static\n", ifaceName))
				builder.WriteString(fmt.Sprintf("    address %s/%d\n", *nic.Address, *nic.Netmasklen))
				if nic.Routes != nil {
					for _, route := range *nic.Routes {
						if route.To == nil || route.Via == nil {
							continue
						}
						if strings.TrimSpace(*route.To) == "default" {
							builder.WriteString(fmt.Sprintf("    gateway %s\n", strings.TrimSpace(*route.Via)))
							continue
						}
						builder.WriteString(fmt.Sprintf("    up ip route add %s via %s dev %s\n", strings.TrimSpace(*route.To), strings.TrimSpace(*route.Via), ifaceName))
					}
				}
			} else {
				builder.WriteString(fmt.Sprintf("iface %s inet dhcp\n", ifaceName))
			}
			builder.WriteString("\n")

			if nic.Nameservers != nil {
				if nic.Nameservers.Addresses != nil {
					for _, addr := range *nic.Nameservers.Addresses {
						addr = strings.TrimSpace(addr)
						if addr != "" {
							resolvLines = append(resolvLines, "nameserver "+addr)
						}
					}
				}
				if nic.Nameservers.Search != nil {
					for _, search := range *nic.Nameservers.Search {
						search = strings.TrimSpace(search)
						if search != "" {
							searchDomains = append(searchDomains, search)
						}
					}
				}
			}
		}
	}

	interfacesPath := filepath.Join(mountPoint, "etc", "network", "interfaces")
	if err := os.MkdirAll(filepath.Dir(interfacesPath), 0755); err != nil {
		return fmt.Errorf("failed to create alpine network directory: %w", err)
	}
	if err := os.WriteFile(interfacesPath, []byte(builder.String()), 0644); err != nil {
		return fmt.Errorf("failed to write alpine interfaces: %w", err)
	}

	if len(resolvLines) > 0 || len(searchDomains) > 0 {
		var resolvBuilder strings.Builder
		if len(searchDomains) > 0 {
			resolvBuilder.WriteString("search " + strings.Join(searchDomains, " ") + "\n")
		}
		for _, line := range resolvLines {
			resolvBuilder.WriteString(line + "\n")
		}
		resolvPath := filepath.Join(mountPoint, "etc", "resolv.conf")
		if err := os.WriteFile(resolvPath, []byte(resolvBuilder.String()), 0644); err != nil {
			return fmt.Errorf("failed to write alpine resolv.conf: %w", err)
		}
	}

	return nil
}

func hostnameForServer(spec api.Server) string {
	name := strings.TrimSpace(spec.Metadata.Name)
	if name != "" {
		return name
	}

	id := strings.TrimSpace(api.ServerID(spec))
	if id != "" {
		return "vm-" + id
	}

	return "marmot"
}

func machineIDForServer(spec api.Server) string {
	if spec.Metadata.Uuid != nil {
		normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(*spec.Metadata.Uuid), "-", ""))
		if len(normalized) == 32 {
			if _, err := hex.DecodeString(normalized); err == nil {
				return normalized
			}
		}
	}

	seed := strings.TrimSpace(api.ServerID(spec))
	if seed == "" {
		seed = strings.TrimSpace(spec.Metadata.Name)
	}
	if seed == "" {
		seed = "marmot"
	}

	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:16])
}

func hostIDBytes(machineID string) []byte {
	hostID := crc32.ChecksumIEEE([]byte(machineID))
	if hostID == 0 {
		hostID = 1
	}

	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, hostID)
	return buf
}

// LVをマウントポイントへマウント
// 戻り値:
//
//	マウンポイントの絶対パス、使用しているループバックデバイス、エラー
func MountVolume(v api.Volume) (string, string, error) {
	volumeID := api.VolumeID(v)
	// パラメータチェック
	if v.Spec.Type == nil {
		return "", "", errors.New("volume type is nil")
	}
	if volumeID == "" {
		return "", "", errors.New("volume id is empty")
	}
	if v.Spec.Path == nil {
		return "", "", errors.New("volume path is nil")
	}
	if *v.Spec.Type == "lvm" {
		if v.Spec.VolumeGroup == nil {
			return "", "", errors.New("volume volumeGroup is nil")
		}
		if v.Spec.LogicalVolume == nil {
			return "", "", errors.New("volume logicalVolume is nil")
		}
	}

	// マウントポイント作成
	mountPoint := fmt.Sprintf("/mnt/%s", volumeID)
	err := os.Mkdir(mountPoint, 0750)
	if err != nil && !os.IsExist(err) {
		err := errors.New("failed mkdir to setup OS-Disk")
		return "", "", err
	}
	slog.Debug("Created mount point", "mountPoint", mountPoint)
	//defer os.RemoveAll(mountPoint)

	var nbdDevice string

	switch *v.Spec.Type {
	case "qcow2":
		// nbdモジュールの存在チェック
		if !isNbdLoaded() {
			// nbdモジュールをロード
			cmd := exec.Command("modprobe", "nbd", "max_part=8")
			err = cmd.Run()
			if err != nil {
				slog.Error("modprobe nbd command failed", "error", err)
				err := errors.New("modprobe nbd failed to setup OS-Disk")
				return "", "", err
			}
		}

		nbdDevice, err = findFreeNbdDevice()
		if err != nil {
			slog.Error("findFreeNbdDevice failed", "error", err)
			err := errors.New("failed to find free nbd device to setup OS-Disk")
			return "", "", err
		}
		slog.Debug("Using volume", "dev", nbdDevice, "path", *v.Spec.Path)

		// QCOW2イメージをループバックデバイスに接続
		cmd := exec.Command("qemu-nbd", "-c", nbdDevice, *v.Spec.Path)
		err = cmd.Run()
		if err != nil {
			slog.Error("qemu-nbd command failed", "error", err, "nbdDevice", nbdDevice, "path", *v.Spec.Path)
			err := errors.New("qemu-nbd failed to setup OS-Disk")
			return "", "", err
		}
		time.Sleep(1 * time.Second) // デバイス作成を待機

		mountCandidates := []string{fmt.Sprintf("%sp1", nbdDevice), nbdDevice}
		// Rocky 9 の GenericCloud イメージのように、GPTで複数パーティション
		// (bios_grub/ESP/boot/root等)を持ち、ルートファイルシステムがパーティション1ではない
		// 構成に対応するため、検出できた場合はルートパーティション(最大サイズのパーティション)を
		// 最優先のマウント候補にする(issue #622)。検出できない場合は従来通りp1→ディスク全体の順で試す。
		if rootPartNum, err := findRootPartitionNumber(nbdDevice); err == nil && rootPartNum != 1 {
			mountCandidates = append([]string{fmt.Sprintf("%sp%d", nbdDevice, rootPartNum)}, mountCandidates...)
		}
		mounted := false
		for _, dev := range mountCandidates {
			debugPrintln("Mounting", "mount", "-t", "ext4", dev, mountPoint)
			cmd = exec.Command("mount", "-t", "ext4", dev, mountPoint)
			err = cmd.Run()
			if err == nil {
				mounted = true
				break
			}

			// filesystem type を固定しない再試行（alpine cloud image、xfs(Rocky)互換）
			cmd = exec.Command("mount", dev, mountPoint)
			err = cmd.Run()
			if err == nil {
				mounted = true
				break
			}

			slog.Warn("mount attempt failed", "device", dev, "mountPoint", mountPoint, "error", err)
		}
		if !mounted {
			_ = exec.Command("qemu-nbd", "--disconnect", nbdDevice).Run()
			_ = os.RemoveAll(mountPoint)
			err := errors.New("mount failed to setup OS-Disk")
			return "", "", err
		}

	case "lvm":
		lvPath := fmt.Sprintf("/dev/%s/%s", *v.Spec.VolumeGroup, *v.Spec.LogicalVolume)
		slog.Debug("Using volume", "lvPath", lvPath)
		lvdev, err := findTargertPartition(lvPath)
		if err != nil {
			slog.Error("FindTargertPartition failed", "error", err)
			_ = os.RemoveAll(mountPoint)
			return "", "", err
		}
		cmd := exec.Command("mount", "-t", "ext4", lvdev, mountPoint)
		err = cmd.Run()
		if err != nil {
			err := errors.New("mount failed to setup OS-Disk")
			_ = os.RemoveAll(mountPoint)
			return "", "", err
		}
	default:
		_ = os.RemoveAll(mountPoint)
		return "", "", fmt.Errorf("unsupported volume type: %s", *v.Spec.Type)
	}

	slog.Debug("Mounted volume", "type", *v.Spec.Type, "mountPoint", mountPoint, "nbdDevice", nbdDevice)
	return mountPoint, nbdDevice, nil
}

// LVをアンマウント
func UnMountVolume(v api.Volume, mountPoint string, nbdDevice string) error {
	// パラメータチェック
	if v.Spec.Type == nil {
		slog.Error("volume type is nil")
		return errors.New("volume type is nil")
	}

	// アンマウント
	cmd := exec.Command("/bin/umount", mountPoint)
	err := cmd.Run()
	if err != nil {
		slog.Error("umount command failed", "error", err, "mountPoint", mountPoint)
		return err
	}
	slog.Debug("Unmounted volume", "type", *v.Spec.Type, "mountPoint", mountPoint)

	switch *v.Spec.Type {
	case "qcow2":
		// デバイスの削除
		cmd = exec.Command("qemu-nbd", "--disconnect", nbdDevice)
		err = cmd.Run()
		if err != nil {
			slog.Error("qemu-nbd --disconnect command failed", "error", err, "nbdDevice", nbdDevice)
			return err
		}

		// nbdモジュールのアンロード
		for attempt := 1; attempt <= 3; attempt++ {
			cmd = exec.Command("modprobe", "-r", "nbd")
			err = cmd.Run()
			if err == nil {
				break
			}

			slog.Warn("modprobe -r nbd command failed", "error", err, "attempt", attempt, "maxAttempts", 3)
			if attempt < 3 {
				time.Sleep(2 * time.Second)
			}
		}
		if err != nil {
			slog.Error("modprobe -r nbd command failed after retries", "error", err)
			return err
		}

	case "lvm":
		for i := 0; i < 3; i++ {
			// *v.Spec.LogicalVolumeの名前の - を -- に変換して、kpartx -d コマンドでデバイスマップを削除する
			lvName := strings.Replace(*v.Spec.LogicalVolume, "-", "--", -1)
			lvPath := fmt.Sprintf("/dev/mapper/%s-%s", *v.Spec.VolumeGroup, lvName)
			if _, err := exec.Command("kpartx", "-d", lvPath).CombinedOutput(); err != nil {
				slog.Error("kpartx -d command failed", "error", err, "lvPath", lvPath)
				//return err
			}
			time.Sleep(5 * time.Second) // 少し待つ
		}

	default:
		return fmt.Errorf("unsupported volume type: %s", *v.Spec.Type)
	}

	// マウントポイント削除
	err = os.RemoveAll(mountPoint)
	if err != nil {
		return err
	}

	return nil
}

// NetplanConfig は Netplan のルート構造体
type NetplanConfig struct {
	Network Network `yaml:"network"`
}

type Network struct {
	Version   int                 `yaml:"version"`
	Renderer  string              `yaml:"renderer,omitempty"`
	Ethernets map[string]Ethernet `yaml:"ethernets,omitempty"`
	Bridges   map[string]Bridge   `yaml:"bridges,omitempty"`
}

type Ethernet struct {
	Addresses   []string   `yaml:"addresses,omitempty"`
	DHCP4       bool       `yaml:"dhcp4"`
	DHCP6       bool       `yaml:"dhcp6"`
	AcceptRA    *bool      `yaml:"accept-ra,omitempty"`
	Routes      []Route    `yaml:"routes,omitempty"`
	Nameservers Nameserver `yaml:"nameservers,omitempty"`
}

type Bridge struct {
	Interfaces  []string         `yaml:"interfaces,omitempty"`
	Addresses   []string         `yaml:"addresses,omitempty"`
	DHCP4       bool             `yaml:"dhcp4"`
	DHCP6       bool             `yaml:"dhcp6"`
	Routes      []Route          `yaml:"routes,omitempty"`
	Nameservers Nameserver       `yaml:"nameservers,omitempty"`
	Parameters  BridgeParameters `yaml:"parameters,omitempty"`
}

type BridgeParameters struct {
	STP bool `yaml:"stp"`
}

type Route struct {
	To  string `yaml:"to"`
	Via string `yaml:"via"`
}

type Nameserver struct {
	Addresses []string `yaml:"addresses,omitempty"`
	Search    []string `yaml:"search,omitempty"`
}

// NICの設定
func CreateNetplanInterfaces(requestConfig []api.NetworkInterface, mountPoint string) error {

	nicName := []string{"enp1s0", "enp2s0", "enp7s0", "enp8s0", "enp9s0", "enp10s0"}

	config := NetplanConfig{
		Network: Network{
			Version:  2,
			Renderer: "networkd",
		},
	}

	// ネットワーク設定がない場合は、デフォルトネットワークにつないで、 DHCPでIPアドレスを取得する設定にする
	if len(requestConfig) == 0 {
		ethCfg := Ethernet{}
		ethCfg.DHCP4 = true
		ethCfg.DHCP6 = true
		config.Network.Ethernets = make(map[string]Ethernet)
		config.Network.Ethernets[nicName[0]] = ethCfg
	} else {
		for idx, nic := range requestConfig {
			ethCfg := Ethernet{}
			ifaceName := nicName[idx]
			slog.Debug("Configuring interface", "index", idx, "ifaceName", ifaceName, "nic", nic)

			// IPアドレスの設定が無ければ、DHCPでIPアドレスを取得する設定にする
			debugPrintln("DEBUG ===============================================================")
			json, err := json.MarshalIndent(nic, "", "  ")
			if err != nil {
				slog.Error("json.MarshalIndent()", "err", err)
			} else {
				debugPrintln("NIC情報(nic): ", string(json))
			}
			debugPrintln("=====================================================================")

			// IPアドレスとネットマスク長があれば、DHCPは無効にする
			if nic.Address != nil && nic.Netmasklen != nil {
				slog.Debug("IP address is specified in the request, skipping DHCP configuration", "ip address", *nic.Address, "netmask length", *nic.Netmasklen)
				ethCfg.DHCP4 = false
				ethCfg.DHCP6 = false
				addr := fmt.Sprintf("%s/%d", *nic.Address, *nic.Netmasklen)
				ethCfg.Addresses = append(ethCfg.Addresses, addr)
			} else {
				slog.Debug("IP address is not specified in the request, enabling DHCP", "ip address", nic.Address, "netmask length", nic.Netmasklen)
				ethCfg.DHCP4 = OrDefault(nic.Dhcp4, true)
				ethCfg.DHCP6 = OrDefault(nic.Dhcp6, true)
			}
			// dhcp6が無効な場合、RA経由のSLAACでIPv6アドレスが付与されないようにする
			if !ethCfg.DHCP6 {
				ethCfg.AcceptRA = BoolPtr(false)
			}

			// ルート設定 (IPv4/IPv6共通)
			if nic.Routes != nil {
				for _, r := range *nic.Routes {
					if r.To == nil || r.Via == nil {
						return fmt.Errorf("route requires both to and via on interface %s", ifaceName)
					}
					route := Route{
						To:  *r.To,
						Via: *r.Via,
					}
					if err := validateNetplanRoute(nic, route, ifaceName); err != nil {
						return err
					}
					ethCfg.Routes = append(ethCfg.Routes, route)
				}
			}

			// ネームサーバ設定 (IPv4/IPv6共通)
			if nic.Nameservers != nil {
				if nic.Nameservers.Addresses != nil {
					for _, addr := range *nic.Nameservers.Addresses {
						ethCfg.Nameservers.Addresses = append(ethCfg.Nameservers.Addresses, addr)
					}
				}
				if nic.Nameservers.Search != nil {
					for _, search := range *nic.Nameservers.Search {
						ethCfg.Nameservers.Search = append(ethCfg.Nameservers.Search, search)
					}
				}
			}

			if config.Network.Ethernets == nil {
				config.Network.Ethernets = make(map[string]Ethernet)
			}
			config.Network.Ethernets[ifaceName] = ethCfg
		}
	}
	// ネットワーク設定がない場合は、デフォルトネットワークにつないで、 DHCPでIPアドレスを取得する設定にする
	//if len(requestConfig) == 0 {
	//	ethCfg := Ethernet{}
	//	ethCfg.DHCP4 = true
	//	ethCfg.DHCP6 = true
	//	config.Network.Ethernets = make(map[string]Ethernet)
	//	config.Network.Ethernets[nicName[0]] = ethCfg
	//}

	// YAML への変換
	data, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("marshal netplan config failed: %w", err)
	}

	// YAMLファイルへの書き出し
	// Netplan は権限に厳しいため 0600 (所有者のみ読み書き) で保存するのが一般的です
	// 書き込み先のパスが合っていない
	filePath := filepath.Join(mountPoint, "etc", "netplan", "00-nic.yaml")
	err = os.WriteFile(filePath, data, 0600)
	if err != nil {
		return fmt.Errorf("write netplan config failed: %w", err)
	}

	debugPrintln(fmt.Sprintf("Generated %s successfully:\n\n%s", filePath, string(data)))

	return nil
}

// CreateIfupdownInterfaces は Debian 11(bullseye)向けの NIC 設定を ifupdown 形式
// (/etc/network/interfaces.d/<interface名>、拡張子無し)で書き出す。Debian 11 の
// GenericCloud イメージには netplan が含まれておらず、ifupdown(resolvconf 併用)で
// ネットワークを管理するため、CreateNetplanInterfaces は適用されない(issue #622)。
// NIC名の割り当てや静的アドレス指定時の挙動は CreateNetplanInterfaces と揃える。
//
// ベースイメージの /etc/network/interfaces は source-directory で
// /etc/network/interfaces.d を読み込む設定になっているため、ここで書き込んだファイルは
// 追加設定として反映される。source-directory はファイル名が英数字・アンダースコア・
// ハイフンのみのものに限り読み込む仕様のため、ファイル名に拡張子(ドット)を付けてはならない
// (付けると無視されて設定が適用されない、issue #622)。また resolvconf がインストール済みの
// ため、dns-nameservers/dns-search ディレクティブで /etc/resolv.conf が自動生成される。
func CreateIfupdownInterfaces(requestConfig []api.NetworkInterface, mountPoint string) error {
	nicName := []string{"enp1s0", "enp2s0", "enp7s0", "enp8s0", "enp9s0", "enp10s0"}

	interfacesDir := filepath.Join(mountPoint, "etc", "network", "interfaces.d")
	if err := os.MkdirAll(interfacesDir, 0755); err != nil {
		return fmt.Errorf("failed to create interfaces.d directory: %w", err)
	}

	// ネットワーク設定がない場合は、デフォルトネットワークにつないで、DHCPでIPアドレスを取得する設定にする
	if len(requestConfig) == 0 {
		return writeIfupdownInterfaceFile(interfacesDir, nicName[0], ifupdownInterfaceConfig{dhcp4: true, dhcp6: true})
	}

	for idx, nic := range requestConfig {
		ifaceName := nicName[idx]
		cfg := ifupdownInterfaceConfig{}

		// IPアドレスとネットマスク長があれば、DHCPは無効にする(CreateNetplanInterfacesと同様)
		if nic.Address != nil && nic.Netmasklen != nil {
			cfg.address = fmt.Sprintf("%s/%d", *nic.Address, *nic.Netmasklen)
			cfg.addressIsIPv6 = checkIPVersion(strings.TrimSpace(*nic.Address)) == "IPv6"
		} else {
			cfg.dhcp4 = OrDefault(nic.Dhcp4, true)
			cfg.dhcp6 = OrDefault(nic.Dhcp6, true)
		}

		if nic.Routes != nil {
			for _, r := range *nic.Routes {
				if r.To == nil || r.Via == nil {
					return fmt.Errorf("route requires both to and via on interface %s", ifaceName)
				}
				route := Route{To: *r.To, Via: *r.Via}
				if err := validateNetplanRoute(nic, route, ifaceName); err != nil {
					return err
				}
				cfg.routes = append(cfg.routes, route)
			}
		}

		if nic.Nameservers != nil {
			if nic.Nameservers.Addresses != nil {
				cfg.dnsAddresses = append(cfg.dnsAddresses, (*nic.Nameservers.Addresses)...)
			}
			if nic.Nameservers.Search != nil {
				cfg.dnsSearch = append(cfg.dnsSearch, (*nic.Nameservers.Search)...)
			}
		}

		if err := writeIfupdownInterfaceFile(interfacesDir, ifaceName, cfg); err != nil {
			return err
		}
	}

	return nil
}

type ifupdownInterfaceConfig struct {
	address       string
	addressIsIPv6 bool
	dhcp4         bool
	dhcp6         bool
	routes        []Route
	dnsAddresses  []string
	dnsSearch     []string
}

func writeIfupdownInterfaceFile(interfacesDir, ifaceName string, cfg ifupdownInterfaceConfig) error {
	var b strings.Builder
	b.WriteString("# Managed by marmot. Do not edit manually (issue #622).\n")
	b.WriteString(fmt.Sprintf("auto %s\n", ifaceName))

	switch {
	case cfg.address != "":
		family := "inet"
		if cfg.addressIsIPv6 {
			family = "inet6"
		}
		b.WriteString(fmt.Sprintf("iface %s %s static\n", ifaceName, family))
		b.WriteString(fmt.Sprintf("    address %s\n", cfg.address))

		var upCommands []string
		for _, route := range cfg.routes {
			if strings.EqualFold(strings.TrimSpace(route.To), "default") {
				b.WriteString(fmt.Sprintf("    gateway %s\n", route.Via))
				continue
			}
			upCommands = append(upCommands, fmt.Sprintf("    up ip route add %s via %s dev %s\n", route.To, route.Via, ifaceName))
		}

		if len(cfg.dnsAddresses) > 0 {
			b.WriteString(fmt.Sprintf("    dns-nameservers %s\n", strings.Join(cfg.dnsAddresses, " ")))
		}
		if len(cfg.dnsSearch) > 0 {
			b.WriteString(fmt.Sprintf("    dns-search %s\n", strings.Join(cfg.dnsSearch, " ")))
		}
		for _, up := range upCommands {
			b.WriteString(up)
		}
	default:
		if cfg.dhcp4 {
			b.WriteString(fmt.Sprintf("iface %s inet dhcp\n", ifaceName))
		}
		if cfg.dhcp6 {
			b.WriteString(fmt.Sprintf("iface %s inet6 dhcp\n", ifaceName))
		}
	}

	// ifupdown の source-directory は、ファイル名が英数字・アンダースコア・ハイフンのみで
	// 構成されるものに限り読み込む仕様のため、ドットを含むファイル名(例: "enp1s0.cfg")は
	// 無視され、設定が適用されない(issue #622)。そのため拡張子を付けずインターフェース名を
	// そのままファイル名にする。
	filePath := filepath.Join(interfacesDir, ifaceName)
	if err := os.WriteFile(filePath, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("failed to write ifupdown interface file %s: %w", filePath, err)
	}

	debugPrintln(fmt.Sprintf("Generated %s successfully:\n\n%s", filePath, b.String()))

	return nil
}

func validateNetplanRoute(nic api.NetworkInterface, route Route, ifaceName string) error {
	to := strings.TrimSpace(route.To)
	via := strings.TrimSpace(route.Via)
	if to == "" || via == "" {
		return fmt.Errorf("route requires non-empty to and via on interface %s", ifaceName)
	}
	if !strings.EqualFold(to, "default") {
		return nil
	}
	if nic.Address == nil || nic.Netmasklen == nil {
		return nil
	}

	addr, err := netip.ParseAddr(strings.TrimSpace(*nic.Address))
	if err != nil {
		return fmt.Errorf("invalid interface address %q on interface %s: %w", *nic.Address, ifaceName, err)
	}
	prefix := netip.PrefixFrom(addr, *nic.Netmasklen).Masked()
	viaAddr, err := netip.ParseAddr(via)
	if err != nil {
		return fmt.Errorf("invalid route gateway %q on interface %s: %w", via, ifaceName, err)
	}
	if viaAddr == prefix.Addr() {
		return fmt.Errorf("default route gateway %s on interface %s must not be the network address %s", via, ifaceName, prefix.Addr())
	}

	return nil
}

// CreateNetworkManagerKeyfiles は Rocky Linux (NetworkManager) 向けの NIC 設定を
// keyfile 形式(/etc/NetworkManager/system-connections/*.nmconnection)で書き出す。
// NIC名の割り当てや静的アドレス指定時の挙動は CreateNetplanInterfaces と揃える。
//
// Rocky Linux の GenericCloud イメージは net.ifnames=0 (従来の eth0/eth1 命名)が既定であり、
// Ubuntu の cloud image が前提とする PCIスロットベースの命名(enp1s0 等)にはならない
// (enp1s0 等は udev の altname としてのみ残る)。そのため接続プロファイルを interface-name
// でマッチさせると実デバイスに一致せず適用されない。MACアドレスが分かっている場合は
// interface-name の代わりに mac-address でマッチさせることで、実際のカーネル命名に
// 依存せず正しいNICへ適用されるようにする(issue #622)。
func CreateNetworkManagerKeyfiles(requestConfig []api.NetworkInterface, mountPoint string) error {
	nicName := []string{"enp1s0", "enp2s0", "enp7s0", "enp8s0", "enp9s0", "enp10s0"}

	// Rocky Linux 8 の GenericCloud イメージは ifcfg-rh プラグイン向けの
	// /etc/sysconfig/network-scripts/ifcfg-eth0 等を同梱しており、NetworkManager の
	// ifcfg-rh プラグインが既定で有効なため、ここで書き込む keyfile 接続と競合しうる
	// (Rocky 9/AlmaLinux 9 の GenericCloud イメージにはこれらのファイルは無い)。
	// keyfile 接続を一意の設定として確実に適用するため、残存する legacy ifcfg-* を
	// 事前に削除する(issue #622)。
	if err := removeLegacyIfcfgNetworkScripts(mountPoint); err != nil {
		return err
	}

	connDir := filepath.Join(mountPoint, "etc", "NetworkManager", "system-connections")
	if err := os.MkdirAll(connDir, 0755); err != nil {
		return fmt.Errorf("failed to create NetworkManager system-connections directory: %w", err)
	}

	// ネットワーク設定がない場合は、デフォルトネットワークにつないで、DHCPでIPアドレスを取得する設定にする
	if len(requestConfig) == 0 {
		return writeNMConnectionFile(connDir, nicName[0], nmConnectionConfig{dhcp4: true, dhcp6: true})
	}

	for idx, nic := range requestConfig {
		ifaceName := nicName[idx]
		cfg := nmConnectionConfig{}

		if nic.Mac != nil {
			if mac := strings.TrimSpace(*nic.Mac); mac != "" {
				cfg.mac = mac
			}
		}

		// IPアドレスとネットマスク長があれば、DHCPは無効にする(CreateNetplanInterfacesと同様)
		if nic.Address != nil && nic.Netmasklen != nil {
			cfg.address = fmt.Sprintf("%s/%d", *nic.Address, *nic.Netmasklen)
			cfg.addressIsIPv6 = checkIPVersion(strings.TrimSpace(*nic.Address)) == "IPv6"
		} else {
			cfg.dhcp4 = OrDefault(nic.Dhcp4, true)
			cfg.dhcp6 = OrDefault(nic.Dhcp6, true)
		}

		if nic.Routes != nil {
			for _, r := range *nic.Routes {
				if r.To == nil || r.Via == nil {
					return fmt.Errorf("route requires both to and via on interface %s", ifaceName)
				}
				route := Route{To: *r.To, Via: *r.Via}
				if err := validateNetplanRoute(nic, route, ifaceName); err != nil {
					return err
				}
				cfg.routes = append(cfg.routes, route)
			}
		}

		if nic.Nameservers != nil {
			if nic.Nameservers.Addresses != nil {
				cfg.dnsAddresses = append(cfg.dnsAddresses, (*nic.Nameservers.Addresses)...)
			}
			if nic.Nameservers.Search != nil {
				cfg.dnsSearch = append(cfg.dnsSearch, (*nic.Nameservers.Search)...)
			}
		}

		if err := writeNMConnectionFile(connDir, ifaceName, cfg); err != nil {
			return err
		}
	}

	return nil
}

// removeLegacyIfcfgNetworkScripts は /etc/sysconfig/network-scripts/ 配下の
// legacy ifcfg-* ファイル(ifcfg-lo を除く)を削除する。NetworkManager の
// ifcfg-rh プラグインが既定で有効な EL8 系イメージ(Rocky Linux 8 等)では、
// これらのファイルが起動時に接続プロファイルとして読み込まれ、ここで書き込む
// keyfile 接続と競合するため、事前に取り除く(issue #622)。対象ディレクトリや
// ファイルが無い場合は何もしない(EL9 系イメージ等)。
func removeLegacyIfcfgNetworkScripts(mountPoint string) error {
	scriptsDir := filepath.Join(mountPoint, "etc", "sysconfig", "network-scripts")
	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read legacy network-scripts directory: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "ifcfg-") || name == "ifcfg-lo" {
			continue
		}
		if err := os.Remove(filepath.Join(scriptsDir, name)); err != nil {
			return fmt.Errorf("failed to remove legacy network script %s: %w", name, err)
		}
	}

	return nil
}

// nmConnectionConfig は1つの NetworkManager keyfile 接続プロファイルを組み立てるための中間表現。
type nmConnectionConfig struct {
	mac           string
	address       string
	addressIsIPv6 bool
	dhcp4         bool
	dhcp6         bool
	routes        []Route
	dnsAddresses  []string
	dnsSearch     []string
}

// writeNMConnectionFile は nmConnectionConfig から NetworkManager keyfile を生成し、
// mountPoint配下の connDir に書き込む。NetworkManagerはパーミッションに厳しいため
// 0600 (所有者のみ読み書き) で保存する。
func writeNMConnectionFile(connDir, ifaceName string, cfg nmConnectionConfig) error {
	var b strings.Builder

	b.WriteString("[connection]\n")
	fmt.Fprintf(&b, "id=%s\n", ifaceName)
	fmt.Fprintf(&b, "uuid=%s\n", uuid.NewString())
	b.WriteString("type=ethernet\n")
	if cfg.mac == "" {
		// MACアドレスが分からない場合のみ、従来通りインターフェース名でマッチさせる
		// (net.ifnames=0 環境では実デバイス名と一致せず適用されないため、可能な限り
		// mac-address でのマッチを優先する)。
		fmt.Fprintf(&b, "interface-name=%s\n", ifaceName)
	}
	b.WriteString("autoconnect=true\n\n")

	b.WriteString("[ethernet]\n")
	if cfg.mac != "" {
		fmt.Fprintf(&b, "mac-address=%s\n", cfg.mac)
	}
	b.WriteString("\n")

	ipv4Routes, ipv6Routes := splitRoutesByFamily(cfg.routes)
	ipv4DNS, ipv6DNS := splitAddressesByFamily(cfg.dnsAddresses)

	b.WriteString("[ipv4]\n")
	switch {
	case cfg.address != "" && !cfg.addressIsIPv6:
		b.WriteString("method=manual\n")
		fmt.Fprintf(&b, "address1=%s\n", cfg.address)
	case cfg.address != "" && cfg.addressIsIPv6:
		b.WriteString("method=disabled\n")
	case cfg.dhcp4:
		b.WriteString("method=auto\n")
	default:
		b.WriteString("method=disabled\n")
	}
	writeNMRoutes(&b, ipv4Routes)
	writeNMDNS(&b, ipv4DNS, cfg.dnsSearch)
	b.WriteString("\n")

	b.WriteString("[ipv6]\n")
	switch {
	case cfg.address != "" && cfg.addressIsIPv6:
		b.WriteString("method=manual\n")
		fmt.Fprintf(&b, "address1=%s\n", cfg.address)
	case cfg.address != "" && !cfg.addressIsIPv6:
		b.WriteString("method=disabled\n")
	case cfg.dhcp6:
		b.WriteString("method=auto\n")
	default:
		b.WriteString("method=disabled\n")
	}
	writeNMRoutes(&b, ipv6Routes)
	writeNMDNS(&b, ipv6DNS, nil)
	b.WriteString("\n")

	filePath := filepath.Join(connDir, ifaceName+".nmconnection")
	if err := os.WriteFile(filePath, []byte(b.String()), 0600); err != nil {
		return fmt.Errorf("write NetworkManager connection file failed: %w", err)
	}

	debugPrintln(fmt.Sprintf("Generated %s successfully:\n\n%s", filePath, b.String()))

	return nil
}

// splitRoutesByFamily はゲートウェイ(via)のアドレス種別でルートをIPv4/IPv6に分類する。
// "default" の宛先は NetworkManager keyfile 形式の表記(0.0.0.0/0 または ::/0)に変換する。
func splitRoutesByFamily(routes []Route) ([]Route, []Route) {
	var ipv4, ipv6 []Route
	for _, r := range routes {
		isIPv6 := checkIPVersion(strings.TrimSpace(r.Via)) == "IPv6"
		to := strings.TrimSpace(r.To)
		if strings.EqualFold(to, "default") {
			if isIPv6 {
				to = "::/0"
			} else {
				to = "0.0.0.0/0"
			}
		}
		route := Route{To: to, Via: r.Via}
		if isIPv6 {
			ipv6 = append(ipv6, route)
		} else {
			ipv4 = append(ipv4, route)
		}
	}
	return ipv4, ipv6
}

// splitAddressesByFamily はネームサーバのアドレスをIPv4/IPv6に分類する。
func splitAddressesByFamily(addresses []string) ([]string, []string) {
	var ipv4, ipv6 []string
	for _, addr := range addresses {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if checkIPVersion(addr) == "IPv6" {
			ipv6 = append(ipv6, addr)
		} else {
			ipv4 = append(ipv4, addr)
		}
	}
	return ipv4, ipv6
}

func writeNMRoutes(b *strings.Builder, routes []Route) {
	for i, r := range routes {
		fmt.Fprintf(b, "route%d=%s,%s\n", i+1, r.To, r.Via)
	}
}

func writeNMDNS(b *strings.Builder, dnsAddresses []string, dnsSearch []string) {
	if len(dnsAddresses) > 0 {
		fmt.Fprintf(b, "dns=%s;\n", strings.Join(dnsAddresses, ";"))
	}
	if len(dnsSearch) > 0 {
		fmt.Fprintf(b, "dns-search=%s;\n", strings.Join(dnsSearch, ";"))
	}
}

// LOOP_CTL_GET_FREE は新しい空きループデバイスを取得するための定数
// 通常、Linuxカーネルでは 0x4C82 です
const LOOP_CTL_GET_FREE = 0x4C82

func getFreeLoopDevice() (string, error) {
	// 1. ループコントロールデバイスを開く
	f, err := os.OpenFile("/dev/loop-control", os.O_RDWR, 0660)
	if err != nil {
		return "", fmt.Errorf("failed to open /dev/loop-control: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	// 2. ioctl システムコールで空き番号を取得
	// 第3引数に 0 を渡すと、未使用のデバイス番号が返ってくる
	index, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		uintptr(LOOP_CTL_GET_FREE),
		0,
	)

	if errno != 0 {
		return "", fmt.Errorf("ioctl failed: %v", errno)
	}

	// 3. パスを組み立てる (例: /dev/loop5)
	return fmt.Sprintf("/dev/loop%d", index), nil
}

func isNbdLoaded() bool {
	// nbdモジュールがロードされると、このディレクトリが作成される
	_, err := os.Stat("/sys/module/nbd")
	return err == nil || !os.IsNotExist(err)
}

func findFreeNbdDevice() (string, error) {
	// 通常、nbdは0番から順にチェックする
	for i := 0; i < 16; i++ {
		devicePath := fmt.Sprintf("/dev/nbd%d", i)
		sysPath := fmt.Sprintf("/sys/class/block/nbd%d/pid", i)

		// /dev/nbdX が存在するか確認
		if _, err := os.Stat(devicePath); os.IsNotExist(err) {
			continue // デバイスファイル自体がない場合は次へ
		}

		// /sys/class/block/nbdX/pid が存在しなければ空いている
		if _, err := os.Stat(sysPath); os.IsNotExist(err) {
			return devicePath, nil
		}
	}
	return "", fmt.Errorf("no free nbd device found")
}

// findRootPartitionNumber は、nbdDevice (例: /dev/nbd0) に接続されているイメージの
// パーティションテーブルを parted で直接読み取り、ルートファイルシステムが入っている
// パーティション番号を求める。マウント対象の決定に使用する(issue #622)。
//
// 当初は「最大のパーティション番号」を対象にしていたが、これは誤りだった。GPTの
// パーティション番号は物理的な並び順やサイズとは無関係に採番される。例えば Ubuntu の
// cloud image はルートパーティションを番号「1」とし、bios_grub/ESP/boot には
// 14/15/16 という番号より大きい番号を割り当てている(ルートは最大番号ではない)。
// そのため「最大番号」を基準にすると、Ubuntuでは /boot (番号16)を誤ってルートとして
// マウントしてしまい、/etc 等が存在せず以降のセットアップが失敗していた。
//
// ルートファイルシステムはディスク上で最も大きいパーティションになるという前提は
// Ubuntu・Rocky 9 のどちらの実イメージでも成立するため、パーティション番号ではなく
// 「最大サイズのパーティション」を選ぶ方式にしている。
//
// sysfsのパーティションデバイスノードはNBDデバイス番号の使い回しで古い情報が残ることがあるため
// 参照せず、オンディスクのパーティションテーブルを直接読む。pkg/marmotd にも同種のロジックが
// あるが、pkg/marmotd は pkg/util に依存しており循環参照になるため、ここに複製している
// (managementNetworkAptCacherAddress/Port と同じ理由。値を変更する場合は両方を同期すること)。
func findRootPartitionNumber(nbdDevice string) (int, error) {
	out, err := exec.Command("parted", "-m", "-s", nbdDevice, "unit", "s", "print").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("parted -m -s %s unit s print failed: %w, output=%s", nbdDevice, err, strings.TrimSpace(string(out)))
	}
	return parseRootPartitionNumberFromPartedOutput(string(out))
}

// parseRootPartitionNumberFromPartedOutput は `parted -m -s <dev> unit s print` の出力から、
// 最もサイズの大きいパーティションの番号を求める。ヘッダ行("BYT;")、ディスク概要行、
// qemu-img resize 直後に表示されるGPT不整合の警告メッセージなどは、いずれも先頭フィールドが
// 数値にならないため自然に無視される。
func parseRootPartitionNumberFromPartedOutput(output string) (int, error) {
	bestNum := 0
	bestSize := int64(-1)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 4 {
			continue
		}
		num, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		size, err := strconv.ParseInt(strings.TrimSuffix(fields[3], "s"), 10, 64)
		if err != nil {
			continue
		}
		if size > bestSize {
			bestSize = size
			bestNum = num
		}
	}
	if bestNum == 0 {
		return 0, fmt.Errorf("no partitions found in parted output")
	}
	return bestNum, nil
}

func findTargertPartition(lvPath string) (string, error) {
	// kpartx でマップ作成
	out, err := exec.Command("kpartx", "-av", lvPath).CombinedOutput()
	if err != nil {
		slog.Error("kpartx -av command failed", "error", err, "lvPath", lvPath, "output", string(out))
		return "", fmt.Errorf("kpartx -av command failed: %v, lvpath: %s, output: %s", err, lvPath, string(out))
	}

	slog.Debug("kpartx output", "output", string(out))
	if len(out) == 0 {
		slog.Error("kpartx -av command returned empty output")
		//exec.Command("kpartx", "-d", lvPath).CombinedOutput()
		return "", fmt.Errorf("kpartx -av command returned empty output")
	}

	// 最後に後片付け としてデバイスマップを削除は、ここで実行しない。unmount時に実行する
	//defer exec.Command("kpartx", "-d", lvPath).Run()

	// 作成されたデバイスリストから目的のものを探す
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "add map") {
			fields := strings.Fields(line)
			deviceNode := "/dev/mapper/" + fields[2]

			// blkid で中身を確認
			check := exec.Command("blkid", deviceNode, "-s", "TYPE", "-o", "value")
			fsType, err := check.Output()
			if err != nil {
				continue
			}
			slog.Debug("Found partition", "deviceNode", deviceNode, "fsType", strings.TrimSpace(string(fsType)))

			if strings.TrimSpace(string(fsType)) == "ext4" {
				return deviceNode, nil
			}
		}
	}

	return "", fmt.Errorf("target partition not found")
}

// CheckIPVersion は文字列からプロトコルバージョンを返します
func checkIPVersion(s string) string {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return "invalid"
	}

	if addr.Is4() {
		return "IPv4"
	} else if addr.Is6() {
		return "IPv6"
	}
	return "unknown"
}
