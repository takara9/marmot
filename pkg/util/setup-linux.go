package util

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

func SetupLinux(spec api.Server) error {
	if spec.BootVolume == nil {
		return fmt.Errorf("BootVolume is nil")
	}

	// ブートボリュームをマウント
	mountPoint, nbdDev, err := MountVolume(*spec.BootVolume)
	if err != nil {
		slog.Error("MountVolume failed", "error", err)
		return err
	}
	defer UnMountVolume(*spec.BootVolume, mountPoint, nbdDev)

	// ホスト名設定
	hostnameFile := filepath.Join(mountPoint, "etc/hostname")
	err = os.WriteFile(hostnameFile, []byte(*spec.Name), 0644)
	if err != nil {
		slog.Error("WriteFile hostname failed", "error", err)
		return err
	}

	// ホストIDの設定
	hostidFile := filepath.Join(mountPoint, "etc/machine-id")
	err = os.WriteFile(hostidFile, []byte(spec.Id), 0644)
	if err != nil {
		slog.Error("WriteFile /etc/machine-id failed", "error", err)
		return err
	}

	// ネットワーク設定
	if spec.Network == nil {
		// ネットワーク設定がない場合は、デフォルトネットワークにつないで、 DHCPでIPアドレスを取得する設定にする
		defaultNic := api.Network{
			Networkname: StringPtr("default"),
			Dhcp4:       BoolPtr(true),
			Dhcp6:       BoolPtr(false),
		}
		spec.Network = &[]api.Network{defaultNic}
	}

	if err := CreateNetplanInterfaces(*spec.Network, mountPoint); err != nil {
		slog.Error("CreateNetplanInterfaces failed", "error", err)
		return err
	}

	return nil
}

// LVをマウントポイントへマウント
// 戻り値:
//
//	マウンポイントの絶対パス、使用しているループバックデバイス、エラー
func MountVolume(v api.Volume) (string, string, error) {
	// パラメータチェック
	if v.Type == nil {
		return "", "", errors.New("volume type is nil")
	}
	if v.Id == "" {
		return "", "", errors.New("volume id is empty")
	}
	if v.Path == nil {
		return "", "", errors.New("volume path is nil")
	}
	if *v.Type == "lvm" {
		if v.VolumeGroup == nil {
			return "", "", errors.New("volume volumeGroup is nil")
		}
		if v.LogicalVolume == nil {
			return "", "", errors.New("volume logicalVolume is nil")
		}
	}

	// マウントポイント作成
	mountPoint := fmt.Sprintf("/mnt/%s", v.Id)
	err := os.Mkdir(mountPoint, 0750)
	if err != nil && !os.IsExist(err) {
		err := errors.New("failed mkdir to setup OS-Disk")
		return "", "", err
	}
	slog.Debug("Created mount point", "mountPoint", mountPoint)

	var nbdDevice string

	switch *v.Type {
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
		slog.Debug("Using volume", "dev", nbdDevice, "path", *v.Path)

		// QCOW2イメージをループバックデバイスに接続
		cmd := exec.Command("qemu-nbd", "-c", nbdDevice, *v.Path)
		err = cmd.Run()
		if err != nil {
			slog.Error("qemu-nbd command failed", "error", err, "nbdDevice", nbdDevice, "path", *v.Path)
			err := errors.New("qemu-nbd failed to setup OS-Disk")
			return "", "", err
		}

		fmt.Println("Mounting", "mount", "-t", "ext4", fmt.Sprintf("%sp1", nbdDevice), mountPoint)
		time.Sleep(3 * time.Second) // 少し待つ

		// ループバックデバイスの2番パーティションをマウント
		cmd = exec.Command("mount", "-t", "ext4", fmt.Sprintf("%sp1", nbdDevice), mountPoint)
		err = cmd.Run()
		if err != nil {
			slog.Error("mount command failed", "error", err, "device", fmt.Sprintf("%sp1", nbdDevice), "mountPoint", mountPoint)
			err := errors.New("mount failed to setup OS-Disk")
			return "", "", err
		}

	case "lvm":
		lvPath := fmt.Sprintf("/dev/%s/%s", *v.VolumeGroup, *v.LogicalVolume)
		lvdev, err := findTargertPartition(lvPath)
		if err != nil {
			slog.Error("FindTargertPartition failed", "error", err)
			return "", "", err
		}
		cmd := exec.Command("mount", "-t", "ext4", lvdev, mountPoint)
		err = cmd.Run()
		if err != nil {
			err := errors.New("mount failed to setup OS-Disk")
			return "", "", err
		}
	default:
		return "", "", fmt.Errorf("unsupported volume type: %s", *v.Type)
	}

	slog.Debug("Mounted volume", "type", *v.Type, "mountPoint", mountPoint, "nbdDevice", nbdDevice)
	return mountPoint, nbdDevice, nil
}

// LVをアンマウント
func UnMountVolume(v api.Volume, mountPoint string, nbdDevice string) error {
	// パラメータチェック
	if v.Type == nil {
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

	switch *v.Type {
	case "qcow2":
		// デバイスの削除
		cmd = exec.Command("qemu-nbd", "--disconnect", nbdDevice)
		err = cmd.Run()
		if err != nil {
			slog.Error("qemu-nbd --disconnect command failed", "error", err, "nbdDevice", nbdDevice)
			return err
		}

		// nbdモジュールのアンロード
		cmd = exec.Command("modprobe", "-r", "nbd")
		err = cmd.Run()
		if err != nil {
			slog.Error("modprobe -r nbd command failed", "error", err)
			return err
		}

	case "lvm":
		lvPath := fmt.Sprintf("/dev/mapper/%s-%s", *v.VolumeGroup, *v.LogicalVolume)
		if _, err := exec.Command("kpartx", "-d", lvPath).CombinedOutput(); err != nil {
			slog.Error("kpartx -d command failed", "error", err)
			return err
		}

	default:
		return fmt.Errorf("unsupported volume type: %s", *v.Type)
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
}

type Ethernet struct {
	Addresses   []string   `yaml:"addresses,omitempty"`
	DHCP4       bool       `yaml:"dhcp4"`
	DHCP6       bool       `yaml:"dhcp6"`
	Routes      []Route    `yaml:"routes,omitempty"`
	Nameservers Nameserver `yaml:"nameservers,omitempty"`
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
func CreateNetplanInterfaces(requestConfig []api.Network, mountPoint string) error {

	nicName := []string{"enp1s0", "enp2s0", "enp7s0", "enp8s0", "enp9s0", "enp10s0"}

	config := NetplanConfig{
		Network: Network{
			Version:  2,
			Renderer: "networkd",
		},
	}

	for idx, nic := range requestConfig {
		ethCfg := Ethernet{}
		ifaceName := nicName[idx]
		slog.Debug("Configuring interface", "ifaceName", ifaceName, "nic", nic)

		// DHCP設定
		if nic.Dhcp4 != nil {
			ethCfg.DHCP4 = *nic.Dhcp4
		} else {
			ethCfg.DHCP4 = true
		}
		if nic.Dhcp6 != nil {
			ethCfg.DHCP6 = *nic.Dhcp6
		} else {
			ethCfg.DHCP6 = true
		}

		// IPアドレス設定 これでは複数のIPを持てない。（それでも良いのか？）
		if nic.Address != nil && nic.Netmask != nil {
			ethCfg.DHCP4 = false
			ethCfg.DHCP6 = false
			addr := *nic.Address + "/" + *nic.Netmask
			ethCfg.Addresses = append(ethCfg.Addresses, addr)
		}

		// ルート設定 (IPv4/IPv6共通)
		if nic.Routes != nil {
			for _, r := range *nic.Routes {
				var route Route
				if r.To != nil || r.Via != nil {
					route = Route{
						To:  *r.To,
						Via: *r.Via,
					}
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

	// ネットワーク設定がない場合は、デフォルトネットワークにつないで、 DHCPでIPアドレスを取得する設定にする
	if len(requestConfig) == 0 {
		ethCfg := Ethernet{}
		ethCfg.DHCP4 = true
		ethCfg.DHCP6 = true
		config.Network.Ethernets = make(map[string]Ethernet)
		config.Network.Ethernets[nicName[0]] = ethCfg
	}

	// YAML への変換
	data, err := yaml.Marshal(&config)
	if err != nil {
		log.Fatalf("Marshal error: %v", err)
	}

	// YAMLファイルへの書き出し
	// Netplan は権限に厳しいため 0600 (所有者のみ読み書き) で保存するのが一般的です
	// 書き込み先のパスが合っていない
	filePath := filepath.Join(mountPoint, "etc", "netplan", "00-nic.yaml")
	err = os.WriteFile(filePath, data, 0600)
	if err != nil {
		log.Fatalf("Write file error: %v", err)
	}

	fmt.Printf("Generated %s successfully:\n\n%s", filePath, string(data))

	return nil
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
	defer f.Close()

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

func findTargertPartition(lvPath string) (string, error) {
	// kpartx でマップ作成
	out, err := exec.Command("kpartx", "-av", lvPath).CombinedOutput()
	if err != nil {
		slog.Error("kpartx -av command failed", "error", err)
		return "", err
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
