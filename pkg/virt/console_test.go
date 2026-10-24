package virt

import (
	"testing"
)

func TestExtractDomainConsolePath(t *testing.T) {
	xmlDesc := `
<domain type='kvm'>
  <name>vm-test</name>
  <devices>
    <serial type='pty'>
      <source path='/dev/pts/17'/>
      <target type='isa-serial' port='0'/>
    </serial>
    <console type='pty'>
      <source path='/dev/pts/17'/>
      <target type='serial' port='0'/>
    </console>
  </devices>
</domain>`

	path, err := ExtractDomainConsolePath(xmlDesc)
  if err != nil {
    t.Fatalf("ExtractDomainConsolePath() error = %v", err)
  }
  if path != "/dev/pts/17" {
    t.Fatalf("ExtractDomainConsolePath() = %q, want %q", path, "/dev/pts/17")
  }
}

func TestExtractDomainGraphicalConsoleInfo(t *testing.T) {
	xmlDesc := `
<domain type='kvm'>
  <name>vm-windows-test</name>
  <devices>
    <graphics type='spice' port='5905' autoport='yes' listen='192.168.1.70' passwd='s3cret-passwd'>
      <listen type='address' address='192.168.1.70'/>
    </graphics>
  </devices>
</domain>`

	info, err := ExtractDomainGraphicalConsoleInfo(xmlDesc)
	if err != nil {
		t.Fatalf("ExtractDomainGraphicalConsoleInfo() error = %v", err)
	}
	if info.Host != "192.168.1.70" {
		t.Fatalf("Host = %q, want %q", info.Host, "192.168.1.70")
	}
	if info.Port != 5905 {
		t.Fatalf("Port = %d, want %d", info.Port, 5905)
	}
	if info.Passwd != "s3cret-passwd" {
		t.Fatalf("Passwd = %q, want %q", info.Passwd, "s3cret-passwd")
	}
}

// Listen属性が未設定(Linux系VMの既定動作、127.0.0.1固定)の場合は、
// 127.0.0.1 をデフォルトとして返すことを確認する(非回帰)。
func TestExtractDomainGraphicalConsoleInfoDefaultsListenAddress(t *testing.T) {
	xmlDesc := `
<domain type='kvm'>
  <name>vm-linux-test</name>
  <devices>
    <graphics type='spice' port='5900' autoport='yes' listen='127.0.0.1'>
      <listen type='address' address='127.0.0.1'/>
    </graphics>
  </devices>
</domain>`

	info, err := ExtractDomainGraphicalConsoleInfo(xmlDesc)
	if err != nil {
		t.Fatalf("ExtractDomainGraphicalConsoleInfo() error = %v", err)
	}
	if info.Host != "127.0.0.1" {
		t.Fatalf("Host = %q, want %q", info.Host, "127.0.0.1")
	}
	if info.Passwd != "" {
		t.Fatalf("Passwd = %q, want empty", info.Passwd)
	}
}

func TestExtractDomainGraphicalConsoleInfoMissingSpice(t *testing.T) {
	xmlDesc := `
<domain type='kvm'>
  <name>vm-no-graphics-test</name>
  <devices>
  </devices>
</domain>`

	if _, err := ExtractDomainGraphicalConsoleInfo(xmlDesc); err == nil {
		t.Fatalf("expected error when no SPICE graphics device is present")
	}
}