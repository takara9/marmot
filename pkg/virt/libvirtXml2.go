package virt

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"libvirt.org/go/libvirt"
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

type DiskSpec struct {
	Dev            string
	Src            string
	Bus            uint
	Type           string
	ISCSITarget    string
	ISCSIHost      string
	ISCSIPort      string
	ISCSIInitiator string
}

type NetSpec struct {
	MAC         string
	Network     string
	PortGroup   string
	PortID      string
	Bridge      string //source内にbridge属性がある場合
	Target      string
	InterfaceID string
	Alias       string
	Vlans       []uint
	IsTrunk     bool
	Bus         uint
}

type ChannelSpec struct {
	Type  string
	Path  string
	Name  string
	Alias string
	Port  uint
}

type ClockSpec struct {
	Name       string
	TickPolicy string
	Present    string
}

type ServerSpec struct {
	UUID         string
	Name         string
	RAM          uint
	CountVCPU    uint
	Machine      string
	DiskSpecs    []DiskSpec
	NetSpecs     []NetSpec
	ChannelSpecs []ChannelSpec
	Clocks       []ClockSpec
	OsName       string
	OsVersion    string
}

type LibVirtEp struct {
	Url string
	Com *libvirt.Connect
}

func NewLibVirtEp(url string) (*LibVirtEp, error) {
	conn, err := libvirt.NewConnect(url)
	if err != nil {
		return nil, err
	}

	return &LibVirtEp{
		Url: url,
		Com: conn,
	}, nil
}

func (lve *LibVirtEp) Close() {
	lve.Com.Close()
}

// libvirt XMLを生成する関数
func CreateDomainXML(vs ServerSpec) *libvirtxml.Domain {
	// This function is intentionally left blank.
	dom := &libvirtxml.Domain{
		Type: "kvm", ID: intPtr(1), Name: vs.Name, UUID: vs.UUID,
		Memory:        &libvirtxml.DomainMemory{Value: vs.RAM, Unit: "KiB"},
		CurrentMemory: &libvirtxml.DomainCurrentMemory{Value: vs.RAM, Unit: "KiB"},
		VCPU:          &libvirtxml.DomainVCPU{Placement: "static", Value: vs.CountVCPU},
		// ライフサイクル設定
		OnPoweroff: "destroy",
		OnReboot:   "restart",
		OnCrash:    "destroy",
		OS: &libvirtxml.DomainOS{
			Type:        &libvirtxml.DomainOSType{Arch: "x86_64", Machine: vs.Machine, Type: "hvm"},
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
	for i, d := range vs.DiskSpecs {
		disk := libvirtxml.DomainDisk{
			Device: "disk",
			Driver: &libvirtxml.DomainDiskDriver{
				Name: "qemu", Type: d.Type, Cache: "none", IO: "native",
			},
			Target:  &libvirtxml.DomainDiskTarget{Dev: d.Dev, Bus: "virtio"},
			Alias:   &libvirtxml.DomainAlias{Name: fmt.Sprintf("virtio-disk%d", i)},
			Address: pciAddr(d.Bus, 0, 0),
		}

		cdrom := libvirtxml.DomainDisk{
			Device: "cdrom",
			Driver: &libvirtxml.DomainDiskDriver{
				Name: "qemu", Type: "raw", Cache: "none", IO: "native",
			},
			Target:  &libvirtxml.DomainDiskTarget{Dev: "sda", Bus: "sata"},
			Alias:   &libvirtxml.DomainAlias{Name: "sata0-0-0"},
			Address: &libvirtxml.DomainAddress{Drive: &libvirtxml.DomainAddressDrive{Controller: uintPtr(0), Bus: uintPtr(0), Target: uintPtr(0), Unit: uintPtr(0)}},
		}

		// ソースの割り当て（Typeに応じて切り替え）
		switch d.Type {
		case "raw":
			disk.Source = &libvirtxml.DomainDiskSource{
				Block: &libvirtxml.DomainDiskSourceBlock{Dev: d.Src},
			}
			dom.Devices.Disks = append(dom.Devices.Disks, disk)

		case "qcow2":
			disk.Source = &libvirtxml.DomainDiskSource{
				File: &libvirtxml.DomainDiskSourceFile{File: d.Src},
			}
			dom.Devices.Disks = append(dom.Devices.Disks, disk)

		case "iso":
			cdrom.Source = &libvirtxml.DomainDiskSource{
				File: &libvirtxml.DomainDiskSourceFile{File: d.Src},
			}
			dom.Devices.Disks = append(dom.Devices.Disks, cdrom)

		case "iscsi":
			// "iscsi" is an attachment source protocol, not a qemu image format.
			// Keep driver format as raw while using network source protocol=iscsi.
			disk.Driver.Type = "raw"
			disk.Source = &libvirtxml.DomainDiskSource{
				Network: &libvirtxml.DomainDiskSourceNetwork{
					Protocol: "iscsi",
					Name:     d.ISCSITarget,
					Hosts: []libvirtxml.DomainDiskSourceHost{
						{Name: d.ISCSIHost, Port: d.ISCSIPort},
					},
					Initiator: &libvirtxml.DomainDiskSourceNetworkInitiator{
						IQN: &libvirtxml.DomainDiskSourceNetworkIQN{Name: d.ISCSIInitiator},
					},
				},
			}
			dom.Devices.Disks = append(dom.Devices.Disks, disk)

		}
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
	for _, n := range vs.NetSpecs {
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
					Network:   n.Network,
					PortGroup: n.PortGroup,
					PortID:    n.PortID,
					Bridge:    n.Bridge,
				},
			}
		} else if n.Bridge != "" {
			// <interface type='bridge'> の場合
			iface.Source = &libvirtxml.DomainInterfaceSource{
				Bridge: &libvirtxml.DomainInterfaceSourceBridge{
					Bridge: n.Bridge,
				},
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

	// チャンネルの生成
	for _, c := range vs.ChannelSpecs {
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
	for _, t := range vs.Clocks {
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

	// libosinfo メタデータの設定
	if vs.OsName != "" && vs.OsVersion != "" {
		metaXML := buildLibosInfoXML(vs.OsName, vs.OsVersion)
		if metaXML != "" {
			dom.Metadata = &libvirtxml.DomainMetadata{XML: metaXML}
		}
	}

	return dom
}

// buildLibosInfoXML は osName と osVersion から libosinfo の XML 文字列を生成する。
func buildLibosInfoXML(osName, osVersion string) string {
	var osID string
	switch strings.ToLower(osName) {
	case "ubuntu":
		osID = fmt.Sprintf("http://ubuntu.com/ubuntu/%s", osVersion)
	default:
		return ""
	}
	return fmt.Sprintf(
		`<libosinfo:libosinfo xmlns:libosinfo="http://libosinfo.org/xmlns/libvirt/domain/1.0"><libosinfo:os id="%s"/></libosinfo:libosinfo>`,
		osID,
	)
}

// 構造体で渡すのが良い
func (l *LibVirtEp) DefineAndStartVM(domain libvirtxml.Domain) (string, error) {
	xmlString, err := domain.Marshal()
	if err != nil {
		fmt.Println("Error marshaling domain XML:", err)
		return "", err
	}

	// Create VM
	dom, err := l.Com.DomainDefineXML(xmlString)
	if err != nil {
		return "", err
	}
	defer dom.Free()

	// Start VM
	err = dom.Create()
	if err != nil && isOVSPortAttachConflict(err) {
		slog.Warn("domain start failed due to ovs port conflict, attempting stale-port cleanup and retry", "err", err)
		cleanupStaleOVSPorts()
		err = dom.Create()
	}
	if err != nil {
		return "", err
	}

	consolePath, err := GetDomainConsolePath(dom)
	if err != nil {
		return "", err
	}

	//オートスタートを設定しないと、HVの再起動からの復帰時、停止している。
	err = dom.SetAutostart(true)
	if err != nil {
		return "", err
	}

	return consolePath, nil
}

func isOVSPortAttachConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unable to add port") && strings.Contains(msg, "already attached to bridge")
}

func cleanupStaleOVSPorts() {
	cmd := exec.Command("ovs-vsctl", "--format=csv", "--data=bare", "--no-headings", "--columns=name,error", "find", "Interface", "error!=\"\"")
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("failed to list ovs interfaces with errors", "err", err, "output", strings.TrimSpace(string(out)))
		return
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		errText := strings.Trim(strings.TrimSpace(parts[1]), "\"")
		if name == "" || errText == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(errText), "no such device") {
			continue
		}

		delCmd := exec.Command("ovs-vsctl", "--if-exists", "del-port", name)
		delOut, delErr := delCmd.CombinedOutput()
		if delErr != nil {
			slog.Warn("failed to remove stale ovs port", "port", name, "err", delErr, "output", strings.TrimSpace(string(delOut)))
			continue
		}
		slog.Debug("removed stale ovs port", "port", name, "reason", errText)
	}
}

func (l *LibVirtEp) ListDomains() ([]string, error) {
	var nameList []string

	doms, err := l.Com.ListAllDomains(libvirt.ConnectListAllDomainsFlags(libvirt.CONNECT_LIST_DOMAINS_ACTIVE))
	if err != nil {
		return nameList, err
	}

	for _, dom := range doms {
		name, err := dom.GetName()
		if err != nil {
			return nameList, err
		}
		nameList = append(nameList, name)
	}
	return nameList, nil
}

func (l *LibVirtEp) DeleteDomain(vmname string) error {
	domain, err := l.Com.LookupDomainByName(vmname)
	if err != nil {
		slog.Error("LookupDomainByName()", "err", err)
		return err
	}

	// ドメインの停止
	err = domain.Destroy()
	if err != nil {
		slog.Error("Destroy()", "err", err)
		return err
	}

	for i := 0; i < 5; i++ {
		_, err := l.Com.LookupDomainByName(vmname)
		if err != nil {
			slog.Error("LookupDomainByName()", "err", err)
			break
		}
		time.Sleep(1 * time.Second)
	}

	// ドメインの削除
	err = domain.Undefine()
	if err != nil {
		slog.Error("Undefine()", "err", err)
		return err
	}

	return nil
}

// 仮想マシンの一時停止
func (l *LibVirtEp) SuspendDomain(vmname string) error {
	domain, err := l.Com.LookupDomainByName(vmname)
	if err != nil {
		return err
	}

	// ドメインの一時停止
	err = domain.Suspend()
	if err != nil {
		return err
	}

	return nil
}

// 仮想マシンの再開
func (l *LibVirtEp) ResumeDomain(vmname string) error {
	domain, err := l.Com.LookupDomainByName(vmname)
	if err != nil {
		return err
	}

	// ドメインの再開
	err = domain.Resume()
	if err != nil {
		return err
	}

	return nil
}

// 仮想マシンの停止
func (l *LibVirtEp) StopDomain(vmname string) error {
	domain, err := l.Com.LookupDomainByName(vmname)
	if err != nil {
		return err
	}

	// ドメインの停止
	err = domain.Destroy()
	if err != nil {
		return err
	}

	// autostart を disable に変更
	err = domain.SetAutostart(false)
	if err != nil {
		return err
	}

	return nil
}

// 仮想マシンの開始
func (l *LibVirtEp) StartDomain(vmname string) error {
	domain, err := l.Com.LookupDomainByName(vmname)
	if err != nil {
		return err
	}

	// ドメインの開始
	err = domain.Create()
	if err != nil {
		return err
	}

	// autostart を有効化
	err = domain.SetAutostart(true)
	if err != nil {
		return err
	}

	return nil
}

func memoryToKiB(value uint, unit string) uint64 {
	u := strings.ToLower(strings.TrimSpace(unit))
	if u == "" || u == "kib" || u == "kb" || u == "k" {
		return uint64(value)
	}

	mul := uint64(1)
	switch u {
	case "mib", "mb", "m":
		mul = 1024
	case "gib", "gb", "g":
		mul = 1024 * 1024
	case "tib", "tb", "t":
		mul = 1024 * 1024 * 1024
	case "b", "bytes":
		mul = 1
		value = value / 1024
	default:
		if strings.HasSuffix(u, "ib") {
			prefix := strings.TrimSuffix(u, "ib")
			pow := 0
			switch prefix {
			case "k":
				pow = 1
			case "m":
				pow = 2
			case "g":
				pow = 3
			case "t":
				pow = 4
			case "p":
				pow = 5
			case "e":
				pow = 6
			}
			if pow > 0 {
				mul = 1
				for i := 1; i < pow; i++ {
					mul *= 1024
				}
			}
		}
	}

	return uint64(value) * mul
}

func memoryMBToKiB(memoryMB int) uint64 {
	if memoryMB <= 0 {
		return 0
	}
	return uint64(memoryMB) * 1024
}

func domainResourceDrift(cfg libvirtxml.Domain, cpu *int, memoryMB *int) bool {
	if cpu != nil && *cpu > 0 {
		currentVCPU := uint(0)
		if cfg.VCPU != nil {
			currentVCPU = cfg.VCPU.Value
		}
		if currentVCPU != uint(*cpu) {
			return true
		}
	}

	if memoryMB != nil && *memoryMB > 0 {
		currentMemKiB := uint64(0)
		if cfg.Memory != nil {
			currentMemKiB = memoryToKiB(cfg.Memory.Value, cfg.Memory.Unit)
		}
		if currentMemKiB != memoryMBToKiB(*memoryMB) {
			return true
		}
	}

	return false
}

func isDomainLive(state libvirt.DomainState) bool {
	switch state {
	case libvirt.DOMAIN_RUNNING, libvirt.DOMAIN_BLOCKED, libvirt.DOMAIN_PAUSED, libvirt.DOMAIN_PMSUSPENDED:
		return true
	default:
		return false
	}
}

// HasDomainResourceDrift reports whether desired CPU/Memory differs from persistent domain XML.
func (l *LibVirtEp) HasDomainResourceDrift(vmname string, cpu *int, memoryMB *int) (bool, error) {
	if cpu == nil && memoryMB == nil {
		return false, nil
	}

	domain, err := l.Com.LookupDomainByName(vmname)
	if err != nil {
		return false, err
	}
	defer domain.Free()

	xml, err := domain.GetXMLDesc(0)
	if err != nil {
		return false, err
	}

	var cfg libvirtxml.Domain
	if err := cfg.Unmarshal(xml); err != nil {
		return false, err
	}

	return domainResourceDrift(cfg, cpu, memoryMB), nil
}

func (l *LibVirtEp) syncDomainVCPU(domain *libvirt.Domain, current uint, desired uint, live bool) (bool, error) {
	if desired == 0 || current == desired {
		return false, nil
	}

	if err := domain.SetVcpusFlags(desired, libvirt.DOMAIN_VCPU_CONFIG); err != nil {
		// libvirt のエラーメッセージ文言に依存せず、max vcpu 更新後に再試行する。
		if maxErr := domain.SetVcpusFlags(desired, libvirt.DOMAIN_VCPU_CONFIG|libvirt.DOMAIN_VCPU_MAXIMUM); maxErr != nil {
			return false, err
		}
		if retryErr := domain.SetVcpusFlags(desired, libvirt.DOMAIN_VCPU_CONFIG); retryErr != nil {
			return false, retryErr
		}
	}

	if live {
		if err := domain.SetVcpusFlags(desired, libvirt.DOMAIN_VCPU_LIVE); err != nil {
			slog.Warn("failed to apply live vcpu change; config is updated and will apply on reboot", "desired", desired, "err", err)
		}
	}

	return true, nil
}

func (l *LibVirtEp) syncDomainMemory(domain *libvirt.Domain, currentKiB uint64, desiredKiB uint64, live bool) (bool, error) {
	if desiredKiB == 0 || currentKiB == desiredKiB {
		return false, nil
	}

	desired := desiredKiB

	if err := domain.SetMemoryFlags(desired, libvirt.DOMAIN_MEM_CONFIG|libvirt.DOMAIN_MEM_MAXIMUM); err != nil {
		slog.Warn("failed to set persistent max memory; trying current memory only", "desiredKiB", desiredKiB, "err", err)
	}
	if err := domain.SetMemoryFlags(desired, libvirt.DOMAIN_MEM_CONFIG); err != nil {
		return false, err
	}

	if live {
		if err := domain.SetMemoryFlags(desired, libvirt.DOMAIN_MEM_LIVE); err != nil {
			slog.Warn("failed to apply live memory change; config is updated and will apply on reboot", "desiredKiB", desiredKiB, "err", err)
		}
	}

	return true, nil
}

// SyncDomainResources compares desired CPU/Memory with current libvirt domain config and applies drift.
func (l *LibVirtEp) SyncDomainResources(vmname string, cpu *int, memoryMB *int) (bool, error) {
	if cpu == nil && memoryMB == nil {
		return false, nil
	}

	domain, err := l.Com.LookupDomainByName(vmname)
	if err != nil {
		return false, err
	}
	defer domain.Free()

	xml, err := domain.GetXMLDesc(0)
	if err != nil {
		return false, err
	}

	var cfg libvirtxml.Domain
	if err := cfg.Unmarshal(xml); err != nil {
		return false, err
	}

	state, _, err := domain.GetState()
	if err != nil {
		return false, err
	}
	live := isDomainLive(state)

	changed := false

	if cpu != nil && *cpu > 0 {
		currentVCPU := uint(0)
		if cfg.VCPU != nil {
			currentVCPU = cfg.VCPU.Value
		}
		vcpuChanged, err := l.syncDomainVCPU(domain, currentVCPU, uint(*cpu), live)
		if err != nil {
			return changed, err
		}
		changed = changed || vcpuChanged
	}

	if memoryMB != nil && *memoryMB > 0 {
		currentMemKiB := uint64(0)
		if cfg.Memory != nil {
			currentMemKiB = memoryToKiB(cfg.Memory.Value, cfg.Memory.Unit)
		}
		desiredMemKiB := memoryMBToKiB(*memoryMB)
		memoryChanged, err := l.syncDomainMemory(domain, currentMemKiB, desiredMemKiB, live)
		if err != nil {
			return changed, err
		}
		changed = changed || memoryChanged
	}

	return changed, nil
}
