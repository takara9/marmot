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