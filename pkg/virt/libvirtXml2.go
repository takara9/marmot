package virt

import (
	"fmt"

	"libvirt.org/go/libvirtxml"
)

// --- ヘルパー関数 ---
func uintPtr(u uint) *uint { return &u }
func intPtr(i int) *int    { return &i }
func pciAddr(b, s, f uint) *libvirtxml.DomainAddress {
	return &libvirtxml.DomainAddress{
		PCI: &libvirtxml.DomainAddressPCI{Domain: uintPtr(0), Bus: uintPtr(b), Slot: uintPtr(s), Function: uintPtr(f)},
	}
}
func stringPtr(s string) *string { return &s }

type diskSpec struct {
	Dev  string
	Src  string
	Bus  uint
	Type string
}

type netSpec struct {
	MAC         string
	Network     string
	PortID      string
	Bridge      string //source内にbridge属性がある場合
	Target      string
	InterfaceID string
	Alias       string
	VlanIDs     []int
	IsTrunk     bool
	Bus         uint
}
type channelSpec struct {
	Type  string
	Path  string
	Name  string
	Alias string
	Port  uint
}

type clockSpec struct {
	Name       string
	TickPolicy string
	Present    string
}

type vmSpec struct {
	uuid         string
	name         string
	ram          uint
	countVCPU    uint
	machine      string
	diskSpecs    []diskSpec
	nets         []netSpec
	channelSpecs []channelSpec
	clocks       []clockSpec
}

// libvirt XMLを生成する関数
func createDomainXML(vs vmSpec) (string, error) {
	// This function is intentionally left blank.
	dom := &libvirtxml.Domain{
		Type: "kvm", ID: intPtr(1), Name: vs.name, UUID: vs.uuid,
		Memory:        &libvirtxml.DomainMemory{Value: vs.ram, Unit: "KiB"},
		CurrentMemory: &libvirtxml.DomainCurrentMemory{Value: vs.ram, Unit: "KiB"},
		VCPU:          &libvirtxml.DomainVCPU{Placement: "static", Value: vs.countVCPU},
		// ライフサイクル設定
		OnPoweroff: "destroy",
		OnReboot:   "restart",
		OnCrash:    "destroy",
		OS: &libvirtxml.DomainOS{
			Type:        &libvirtxml.DomainOSType{Arch: "x86_64", Machine: vs.machine, Type: "hvm"},
			BootDevices: []libvirtxml.DomainBootDevice{{Dev: "hd"}},
		},
		Features: &libvirtxml.DomainFeatureList{
			ACPI: &libvirtxml.DomainFeature{}, APIC: &libvirtxml.DomainFeatureAPIC{},
			VMPort: &libvirtxml.DomainFeatureState{State: "off"},
		},
		CPU: &libvirtxml.DomainCPU{
			Mode:       "host-passthrough",
			Check:      "none",
			Migratable: "on",
		},
		Devices: &libvirtxml.DomainDeviceList{
			Emulator: "/usr/bin/qemu-system-x86_64",
		},
		SecLabel: []libvirtxml.DomainSecLabel{
			{
				Type:    "dynamic",
				Model:   "apparmor",
				Relabel: "yes",
			},
			{
				Type:    "dynamic",
				Model:   "dac",
				Relabel: "yes",
			},
		},
	}

	// --- ディスクの生成 ---
	for i, d := range vs.diskSpecs {
		disk := libvirtxml.DomainDisk{
			Device: "disk",
			Driver: &libvirtxml.DomainDiskDriver{
				Name: "qemu", Type: d.Type, Cache: "none", IO: "native",
			},
			Target:  &libvirtxml.DomainDiskTarget{Dev: d.Dev, Bus: "virtio"},
			Alias:   &libvirtxml.DomainAlias{Name: fmt.Sprintf("virtio-disk%d", i)},
			Address: pciAddr(d.Bus, 0, 0),
		}

		// ソースの割り当て（Typeに応じて切り替え）
		switch d.Type {
		case "raw":
			disk.Source = &libvirtxml.DomainDiskSource{
				Block: &libvirtxml.DomainDiskSourceBlock{Dev: d.Src},
			}
		case "qcow2":
			disk.Source = &libvirtxml.DomainDiskSource{
				File: &libvirtxml.DomainDiskSourceFile{File: d.Src},
			}
		}

		dom.Devices.Disks = append(dom.Devices.Disks, disk)
	}

	// PCIコントローラーの生成
	dom.Devices.Controllers = append(dom.Devices.Controllers, libvirtxml.DomainController{
		Type: "pci", Index: uintPtr(0), Model: "pcie-root", Alias: &libvirtxml.DomainAlias{Name: "pcie.0"},
	})
	for i := uint(1); i <= 12; i++ {
		slot, function := uint(2), i-1
		if i > 8 {
			slot, function = 3, i-9
		}
		mf := ""
		if function == 0 {
			mf = "on"
		}

		dom.Devices.Controllers = append(dom.Devices.Controllers, libvirtxml.DomainController{
			Type: "pci", Index: uintPtr(i), Model: "pcie-root-port",
			Alias:   &libvirtxml.DomainAlias{Name: fmt.Sprintf("pci.%d", i)},
			Address: &libvirtxml.DomainAddress{PCI: &libvirtxml.DomainAddressPCI{Domain: uintPtr(0), Bus: uintPtr(0), Slot: uintPtr(slot), Function: uintPtr(function), MultiFunction: mf}},
		})
	}

	// ネットワークインターフェースの生成
	for _, n := range vs.nets {
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
		if len(n.VlanIDs) > 0 {
			tags := []libvirtxml.DomainInterfaceVLanTag{}
			for _, id := range n.VlanIDs {
				tags = append(tags, libvirtxml.DomainInterfaceVLanTag{ID: uint(id)})
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

	// チャンネルの生成
	for _, c := range vs.channelSpecs {
		channel := libvirtxml.DomainChannel{
			Target: &libvirtxml.DomainChannelTarget{
				VirtIO: &libvirtxml.DomainChannelTargetVirtIO{
					Name:  c.Name,
					State: "disconnected",
				},
			},
			Alias: &libvirtxml.DomainAlias{Name: c.Alias},
			Address: &libvirtxml.DomainAddress{
				VirtioSerial: &libvirtxml.DomainAddressVirtioSerial{
					Controller: uintPtr(0),
					Bus:        uintPtr(0),
					Port:       uintPtr(c.Port),
				},
			},
		}

		// Type に応じて Source を動的に構成
		switch c.Type {
		case "unix":
			channel.Source = &libvirtxml.DomainChardevSource{
				UNIX: &libvirtxml.DomainChardevSourceUNIX{
					Mode: "bind",
					Path: c.Path,
				},
			}
		case "spicevmc":
			// spicevmc は Source の詳細設定が不要（暗黙的に管理される）
			channel.Source = &libvirtxml.DomainChardevSource{}
		}

		dom.Devices.Channels = append(dom.Devices.Channels, channel)
	}

	// グラフィックス設定 (SPICE)
	dom.Devices.Graphics = []libvirtxml.DomainGraphic{
		{
			Spice: &libvirtxml.DomainGraphicSpice{
				Port:     5900,
				AutoPort: "yes",
				// listen属性の設定
				Listen: "127.0.0.1",
				// <listen> 子要素の設定
				Listeners: []libvirtxml.DomainGraphicListener{
					{
						Address: &libvirtxml.DomainGraphicListenerAddress{
							Address: "127.0.0.1",
						},
					},
				},
				// <image compression='off'/> の設定
				Image: &libvirtxml.DomainGraphicSpiceImage{
					Compression: "off",
				},
			},
		},
	}

	// -その他の固定デバイス
	dom.Devices.Serials = []libvirtxml.DomainSerial{{Target: &libvirtxml.DomainSerialTarget{Type: "isa-serial", Port: uintPtr(0)}}}
	dom.Devices.MemBalloon = &libvirtxml.DomainMemBalloon{Model: "virtio", Address: pciAddr(4, 0, 0)}

	// タイマーの生成
	// 1. まず dom.Clock が nil でないことを保証し、Offset を設定
	if dom.Clock == nil {
		dom.Clock = &libvirtxml.DomainClock{
			Offset: "utc", // 一般的なデフォルト値
		}
	}

	// 2. タイマーの生成ループ
	for _, t := range vs.clocks {
		timer := libvirtxml.DomainTimer{Name: t.Name}

		if t.TickPolicy != "" {
			timer.TickPolicy = t.TickPolicy
		}

		if t.Present != "" {
			timer.Present = t.Present
		}

		// 正しくは "Timers" (複数形) です
		dom.Clock.Timer = append(dom.Clock.Timer, timer)
	}

	// XML出力
	xml, err := dom.Marshal()
	if err != nil {
		// エラーハンドリング
		fmt.Println("Error marshaling domain XML:", err)
		return "", err
	}
	return string(xml), nil
}
