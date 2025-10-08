package util

/*
   Ubuntu Linux 設定の関数群

   * Linux ホスト名をOS Volへ書き込み
   * Linux Netplanのファイルへ、IPアドレスなどを設定
   * YAMLのlevelに応じた桁下げのYAML行を出力
   * ローカルのMACアドレスを生成する
   * Libvirt XMLにNICを追加

*/

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/takara9/marmot/api"
	//cf "github.com/takara9/marmot/pkg/config"
	"marmot.io/config"
	"github.com/takara9/marmot/pkg/virt"
)

// Linux ホスト名をOS Volへ書き込み
func LinuxSetup_hostname(vm_root string, hostname string) error {
	hostname_file := filepath.Join(vm_root, "etc/hostname")
	err := os.WriteFile(hostname_file, []byte(hostname), 0644)
	return err
}

// Linux hostidをOS Volへ書き込み
func LinuxSetup_hostid(spec config.VMSpec, vm_root string) error {
	ipb := IPaddrByteArray(spec.PrivateIP)
	hostid_file := filepath.Join(vm_root, "etc/hostid")
	err := os.WriteFile(hostid_file, ipb, 0644)
	return err
}

// Linux hostidをOS Volへ書き込み
func LinuxSetup_hostid2(spec api.VmSpec, vm_root string) error {
	ipb := IPaddrByteArray(*spec.PrivateIp)
	hostid_file := filepath.Join(vm_root, "etc/hostid")
	err := os.WriteFile(hostid_file, ipb, 0644)
	return err
}

// 文字列IPアドレスをバイト配列へ変換
func IPaddrByteArray(ip string) []byte {
	ipi := strings.Split(ip, ".")
	ipb := make([]byte, 4)
	for i, xx := range ipi {
		j, _ := strconv.Atoi(xx)
		ipb[i] = byte(j) & 0x0ff
	}
	return ipb
}

/*
 cnf config.MarmotConfigは、利用していない。そのため削除したい。
 デフォルトGW,DNSなどの設定のためにコンフィグファイルからの設定を取り入れている
*/
// Linux Netplanのファイルへ、IPアドレスなどを設定
func LinuxSetup_createNetplan(spec config.VMSpec, vm_root string) error {
	var netplanFile = "etc/netplan/00-nic.yaml"
	netplanPath := filepath.Join(vm_root, netplanFile)

	f, err := os.Create(netplanPath)
	if err != nil {
		return err
	}
	defer f.Close()

	yaml(0, "network:", f)
	yaml(1, "version: 2", f)
	yaml(1, "ethernets:", f)

	if len(spec.PrivateIP) > 0 {
		yaml(2, "enp6s0:", f)
		yaml(3, "addresses:", f)
		yaml(4, fmt.Sprintf("- %s/%d", spec.PrivateIP, 16), f)
		/*
		   不安定になるので設定しない と思ったが、プライベートだけだとインターネットへのルートが必要
		   そこで、パブリックが無い時だけ設定する
		*/
		if len(spec.PublicIP) == 0 {
			if spec.VMOsVariant == "ubuntu18.04" {
				yaml(3, "gateway4: 172.16.0.1", f)
			} else {
				yaml(3, "routes:", f)
				yaml(4, "- to: default", f)
				yaml(4, "  via: 172.16.0.1", f)
			}
		}
		yaml(3, "nameservers:", f)
		yaml(4, fmt.Sprintf("search: [%v]", "labo.local"), f)
		yaml(4, fmt.Sprintf("addresses: [%v]", "172.16.0.4"), f)
	}

	if len(spec.PublicIP) > 0 {
		yaml(2, "enp7s0:", f)
		yaml(3, "addresses:", f)
		yaml(4, fmt.Sprintf("- %s/%d", spec.PublicIP, 24), f)
		if spec.VMOsVariant == "ubuntu18.04" {
			yaml(3, "gateway4: 192.168.1.1", f)

		} else {
			yaml(3, "routes:", f)
			yaml(4, "- to: default", f)
			yaml(4, "  via: 192.168.1.1", f)
		}
		// 両方を有効にできないので、プライベート側を優先する
		if len(spec.PrivateIP) == 0 {
			yaml(4, fmt.Sprintf("search: [%v]", "labo.local"), f)
			yaml(4, fmt.Sprintf("addresses: [%v]", "192.168.1.4"), f)
		}
	}
	return nil

}

// YAMLのlevelに応じた桁下げのYAML行を出力
func yaml(level int, txt string, f io.Writer) {
	for i := 0; i < level; i++ {
		fmt.Fprintf(f, "  ")
	}
	fmt.Fprintf(f, "%s\n", txt)
}

// ローカルのMACアドレスを生成する
func generateMacAddr() string {
	buf := make([]byte, 6)
	rand.Read(buf)
	// Set the local bit https://en.wikipedia.org/wiki/MAC_address
	buf[0] = 0x02
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5])
}

// Libvirt XMLにNICを追加
func CreateNic(netClass string, vmXml *[]virt.Interface) error {
	var vlan string
	var bus string
	var idev virt.Interface

	switch netClass {
	case "pri":
		vlan = "vlan-1001"
		bus = "0x06"
	case "pub":
		vlan = "vlan-1002"
		bus = "0x07"
	default:
		vlan = "default"
		bus = "0x01"
	}

	idev.Type = "network"
	idev.Mac.Address = generateMacAddr() // Macアドレスを生成
	idev.Source.Network = "ovs-network"
	idev.Source.Portgroup = vlan // VLAN設定
	idev.Model.Type = "virtio"
	idev.Address.Type = "pci"
	idev.Address.Domain = "0x0000"
	idev.Address.Bus = bus // PCIバスの番号設定
	idev.Address.Slot = "0x00"
	idev.Address.Function = "0x0"
	*vmXml = append(*vmXml, idev) // I/Fを追加

	return nil
}

/*
 cnf config.MarmotConfigは、利用していない。そのため削除したい。
 デフォルトGW,DNSなどの設定のためにコンフィグファイルからの設定を取り入れている
*/
// Linux Netplanのファイルへ、IPアドレスなどを設定
func LinuxSetup_createNetplan2(spec api.VmSpec, vm_root string) error {
	var netplanFile = "etc/netplan/00-nic.yaml"
	netplanPath := filepath.Join(vm_root, netplanFile)

	f, err := os.Create(netplanPath)
	if err != nil {
		return err
	}
	defer f.Close()

	yaml(0, "network:", f)
	yaml(1, "version: 2", f)
	yaml(1, "ethernets:", f)

	if spec.PrivateIp != nil {
		if len(*spec.PrivateIp) > 0 {
			yaml(2, "enp6s0:", f)
			yaml(3, "addresses:", f)
			yaml(4, fmt.Sprintf("- %s/%d", *spec.PrivateIp, 16), f)
			/*
			   不安定になるので設定しない と思ったが、プライベートだけだとインターネットへのルートが必要
			   そこで、パブリックが無い時だけ設定する
			*/
		}
		if spec.PublicIp != nil {
			if len(*spec.PublicIp) == 0 {
				if *spec.Ostempvariant == "ubuntu18.04" {
					yaml(3, "gateway4: 172.16.0.1", f)
				} else {
					yaml(3, "routes:", f)
					yaml(4, "- to: default", f)
					yaml(4, "  via: 172.16.0.1", f)
				}
			}
		}
		yaml(3, "nameservers:", f)
		yaml(4, fmt.Sprintf("search: [%v]", "labo.local"), f)
		yaml(4, fmt.Sprintf("addresses: [%v]", "172.16.0.4"), f)
	}

	if spec.PublicIp != nil {
		if len(*spec.PublicIp) > 0 {
			yaml(2, "enp7s0:", f)
			yaml(3, "addresses:", f)
			yaml(4, fmt.Sprintf("- %s/%d", *spec.PublicIp, 24), f)
			if *spec.Ostempvariant == "ubuntu18.04" {
				yaml(3, "gateway4: 192.168.1.1", f)

			} else {
				yaml(3, "routes:", f)
				yaml(4, "- to: default", f)
				yaml(4, "  via: 192.168.1.1", f)
			}
			// 両方を有効にできないので、プライベート側を優先する
			if len(*spec.PrivateIp) == 0 {
				yaml(4, fmt.Sprintf("search: [%v]", "labo.local"), f)
				yaml(4, fmt.Sprintf("addresses: [%v]", "192.168.1.4"), f)
			}
		}
	}
	return nil
}
