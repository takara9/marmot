package virt_test

import (
	"strings"
	"testing"

	"github.com/takara9/marmot/pkg/virt"
)

// TestCreateDomainXML_WindowsBranch は、OsName=="windows" の場合に
// UEFI/TPM2.0/virtio-scsi/QXL/SPICE(パスワード付きLAN Listen) が
// 生成されたXMLに反映されることを確認する。
func TestCreateDomainXML_WindowsBranch(t *testing.T) {
	vs := virt.ServerSpec{
		UUID:      "00000000-0000-0000-0000-000000000002",
		Name:      "vm-windows-test",
		RAM:       1024 * 1024,
		CountVCPU: 2,
		Machine:   "pc-q35-4.2",
		OsName:    "windows",
		OsVersion: "2022",
		DiskSpecs: []virt.DiskSpec{
			{Dev: "vda", Src: "/var/lib/marmot/volumes/windows2022-boot.qcow2", Bus: 3, Type: "qcow2"},
		},
		SpiceListenAddress: "192.168.1.70",
		SpicePasswd:        "s3cret-passwd",
	}

	dom := virt.CreateDomainXML(vs)
	xml, err := dom.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	xmlStr := string(xml)

	if dom.OS.Firmware != "efi" {
		t.Fatalf("expected OS.Firmware = \"efi\", got %q", dom.OS.Firmware)
	}
	if !strings.Contains(xmlStr, `firmware="efi"`) {
		t.Fatalf("UEFI firmware attribute is missing: %s", xmlStr)
	}

	if len(dom.Devices.TPMs) != 1 {
		t.Fatalf("expected exactly 1 TPM device, got %d", len(dom.Devices.TPMs))
	}
	if !strings.Contains(xmlStr, "<tpm model=\"tpm-crb\">") {
		t.Fatalf("TPM device is missing: %s", xmlStr)
	}
	if !strings.Contains(xmlStr, `<backend model="emulator" version="2.0">`) &&
		!strings.Contains(xmlStr, `version="2.0"`) {
		t.Fatalf("TPM emulator backend version=2.0 is missing: %s", xmlStr)
	}

	if len(dom.Devices.Disks) != 1 || dom.Devices.Disks[0].Target.Bus != "scsi" {
		t.Fatalf("expected disk bus = \"scsi\", got %+v", dom.Devices.Disks)
	}
	foundSCSIController := false
	for _, c := range dom.Devices.Controllers {
		if c.Type == "scsi" && c.Model == "virtio-scsi" {
			foundSCSIController = true
		}
	}
	if !foundSCSIController {
		t.Fatalf("virtio-scsi controller is missing: %+v", dom.Devices.Controllers)
	}

	if len(dom.Devices.Videos) != 1 || dom.Devices.Videos[0].Model.Type != "qxl" {
		t.Fatalf("expected QXL video device, got %+v", dom.Devices.Videos)
	}

	if dom.Clock == nil || dom.Clock.Offset != "localtime" {
		t.Fatalf("expected clock offset = \"localtime\", got %+v", dom.Clock)
	}

	spice := dom.Devices.Graphics[0].Spice
	if spice.Listen != "192.168.1.70" {
		t.Fatalf("expected spice listen = \"192.168.1.70\", got %q", spice.Listen)
	}
	if spice.Passwd != "s3cret-passwd" {
		t.Fatalf("expected spice passwd = \"s3cret-passwd\", got %q", spice.Passwd)
	}
}

// TestCreateDomainXML_LinuxUnaffected は、OsName が "windows" 以外（Linux系）の場合に
// 既存の挙動（virtioディスク/SPICE 127.0.0.1固定・パスワードなし/UTCクロック/TPM・QXLなし）が
// 維持されることを確認する（非回帰確認）。
func TestCreateDomainXML_LinuxUnaffected(t *testing.T) {
	vs := virt.ServerSpec{
		UUID:      "00000000-0000-0000-0000-000000000003",
		Name:      "vm-linux-test",
		RAM:       1024 * 1024,
		CountVCPU: 2,
		Machine:   "pc-q35-4.2",
		OsName:    "ubuntu",
		OsVersion: "24.04",
		DiskSpecs: []virt.DiskSpec{
			{Dev: "vda", Src: "/var/lib/marmot/volumes/ubuntu2404-boot.qcow2", Bus: 3, Type: "qcow2"},
		},
	}

	dom := virt.CreateDomainXML(vs)

	if dom.OS.Firmware != "" {
		t.Fatalf("expected OS.Firmware to be empty for Linux, got %q", dom.OS.Firmware)
	}
	if len(dom.Devices.TPMs) != 0 {
		t.Fatalf("expected no TPM device for Linux, got %+v", dom.Devices.TPMs)
	}
	if len(dom.Devices.Videos) != 0 {
		t.Fatalf("expected no explicit video device for Linux, got %+v", dom.Devices.Videos)
	}
	if len(dom.Devices.Disks) != 1 || dom.Devices.Disks[0].Target.Bus != "virtio" {
		t.Fatalf("expected disk bus = \"virtio\" for Linux, got %+v", dom.Devices.Disks)
	}
	for _, c := range dom.Devices.Controllers {
		if c.Type == "scsi" && c.Model == "virtio-scsi" {
			t.Fatalf("virtio-scsi controller must not be added for Linux: %+v", dom.Devices.Controllers)
		}
	}
	if dom.Clock == nil || dom.Clock.Offset != "utc" {
		t.Fatalf("expected clock offset = \"utc\" for Linux, got %+v", dom.Clock)
	}

	spice := dom.Devices.Graphics[0].Spice
	if spice.Listen != "127.0.0.1" {
		t.Fatalf("expected spice listen = \"127.0.0.1\" for Linux, got %q", spice.Listen)
	}
	if spice.Passwd != "" {
		t.Fatalf("expected spice passwd to be empty for Linux, got %q", spice.Passwd)
	}
}
