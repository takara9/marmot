package virt

import (
	"libvirt.org/go/libvirtxml"
)

type FileSystemSpec struct {
	SourceDir string
	TargetDir string
	Access    string
}

type LxcSpec struct {
	DomainType      string
	UUID            string
	Name            string
	RAM             uint
	VCPUs           uint
	Machine         string
	BootFilesystem  string
	FileSystemSpecs []FileSystemSpec
	Nets            []NetSpec
}

func MakeLxcDefinition(vs LxcSpec) *libvirtxml.Domain {
	dom := &libvirtxml.Domain{
		Type:   vs.DomainType,
		Name:   vs.Name,
		Memory: &libvirtxml.DomainMemory{Value: vs.RAM * 1024, Unit: "MiB"},
		OS: &libvirtxml.DomainOS{
			Type: &libvirtxml.DomainOSType{Arch: "x86_64", Type: "exe"},
			Init: "/sbin/init",
		},
		VCPU: &libvirtxml.DomainVCPU{
			Placement: "static",
			Value:     vs.VCPUs,
		},
		Devices: &libvirtxml.DomainDeviceList{
			Emulator: "/usr/lib/libvirt/libvirt_lxc",
			Consoles: []libvirtxml.DomainConsole{
				{Target: &libvirtxml.DomainConsoleTarget{Type: "lxc", Port: uintPtr(0)}},
			},
		},
	}

	for _, fs := range vs.FileSystemSpecs {
		filesystem := libvirtxml.DomainFilesystem{
			AccessMode: "passthrough",
			Source: &libvirtxml.DomainFilesystemSource{
				Mount: &libvirtxml.DomainFilesystemSourceMount{
					Dir: fs.SourceDir,
				},
			},
			Target: &libvirtxml.DomainFilesystemTarget{Dir: fs.TargetDir},
		}
		dom.Devices.Filesystems = append(dom.Devices.Filesystems, filesystem)
	}

	lxcfsPaths := []string{"cpuinfo", "meminfo", "stat", "uptime"}
	for _, p := range lxcfsPaths {
		dom.Devices.Filesystems = append(dom.Devices.Filesystems, libvirtxml.DomainFilesystem{
			AccessMode: "passthrough",
			Source: &libvirtxml.DomainFilesystemSource{
				Mount: &libvirtxml.DomainFilesystemSourceMount{
					Dir: "/var/lib/lxcfs/proc/" + p,
				},
			},
			Target: &libvirtxml.DomainFilesystemTarget{
				Dir: "/proc/" + p,
			},
		})
	}

	// ネットワークインターフェースの生成
	for _, n := range vs.Nets {
		iface := libvirtxml.DomainInterface{
			MAC:   &libvirtxml.DomainInterfaceMAC{Address: n.MAC},
			Model: &libvirtxml.DomainInterfaceModel{Type: "virtio"},
			Alias: &libvirtxml.DomainAlias{Name: n.Alias},
		}

		// ソース設定の動的切り替え
		if n.Network != "" {
			// <interface type='network'> の場合
			iface.Source = &libvirtxml.DomainInterfaceSource{
				Network: &libvirtxml.DomainInterfaceSourceNetwork{
					Network: n.Network,
					PortID:  n.PortID,
					Bridge:  n.Bridge,
				},
			}
		} else if n.Bridge != "" {
			// <interface type='bridge'> の場合
			iface.Source = &libvirtxml.DomainInterfaceSource{
				Bridge: &libvirtxml.DomainInterfaceSourceBridge{Bridge: n.Bridge},
			}
		}

		// Target (vnet等) の指定がある場合
		if n.Target != "" {
			iface.Target = &libvirtxml.DomainInterfaceTarget{Dev: n.Target}
		}

		// VLAN / Trunk 設定
		if len(n.Vlans) > 0 {
			tags := []libvirtxml.DomainInterfaceVLanTag{}
			for _, id := range n.Vlans {
				tags = append(tags, libvirtxml.DomainInterfaceVLanTag{ID: id})
			}
			trunk := ""
			if n.IsTrunk {
				trunk = "yes"
			}
			iface.VLan = &libvirtxml.DomainInterfaceVLan{Trunk: trunk, Tags: tags}
		}

		// VirtualPort (Open vSwitch等) の設定
		if n.InterfaceID != "" {
			iface.VirtualPort = &libvirtxml.DomainInterfaceVirtualPort{
				Params: &libvirtxml.DomainInterfaceVirtualPortParams{
					OpenVSwitch: &libvirtxml.DomainInterfaceVirtualPortParamsOpenVSwitch{
						InterfaceID: n.InterfaceID,
					},
				},
			}
		}

		// PCIアドレス (KVM用)
		if n.Bus != 0 {
			iface.Address = pciAddr(n.Bus, 0, 0)
		}

		dom.Devices.Interfaces = append(dom.Devices.Interfaces, iface)
	}

	// ... Marshal ...
	// XML生成
	//xml, err := dom.Marshal()
	//if err != nil {
	//	log.Fatal("XMLの生成に失敗しました:", err)
	//}
	//fmt.Println(xml)
	//return xml, nil
	return dom
}
