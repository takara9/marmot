package virt_test

import (
	"strings"
	"testing"

	"github.com/takara9/marmot/pkg/virt"
)

func TestCreateDomainXML_ISCSIDisk(t *testing.T) {
	vs := virt.ServerSpec{
		UUID:      "00000000-0000-0000-0000-000000000001",
		Name:      "vm-iscsi-test",
		RAM:       1024 * 1024,
		CountVCPU: 2,
		Machine:   "pc-q35-4.2",
		DiskSpecs: []virt.DiskSpec{
			{
				Dev:            "vdb",
				Bus:            11,
				Type:           "iscsi",
				ISCSITarget:    "iqn.2024-01.com.marmot:target-abcde/0",
				ISCSIHost:      "192.168.1.210",
				ISCSIPort:      "3260",
				ISCSIInitiator: "iqn.2004-10.com.marmot:marmot1",
			},
		},
	}

	dom := virt.CreateDomainXML(vs)
	xml, err := dom.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	xmlStr := string(xml)
	if !strings.Contains(xmlStr, "<source protocol=\"iscsi\" name=\"iqn.2024-01.com.marmot:target-abcde/0\">") {
		t.Fatalf("iscsi source is missing: %s", xmlStr)
	}
	if !strings.Contains(xmlStr, "<host name=\"192.168.1.210\" port=\"3260\"></host>") {
		t.Fatalf("iscsi host/port is missing: %s", xmlStr)
	}
	if !strings.Contains(xmlStr, "<initiator>") || !strings.Contains(xmlStr, "<iqn name=\"iqn.2004-10.com.marmot:marmot1\"></iqn>") {
		t.Fatalf("iscsi initiator is missing: %s", xmlStr)
	}
}
