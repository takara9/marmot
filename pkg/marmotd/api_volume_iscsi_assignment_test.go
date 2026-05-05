package marmotd

import (
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func testHostStatus(nodeName, hostID string, updatedSecondsAgo int) api.HostStatus {
	t := time.Now().Add(-time.Duration(updatedSecondsAgo) * time.Second)
	return api.HostStatus{
		NodeName:    util.StringPtr(nodeName),
		HostId:      util.StringPtr(hostID),
		LastUpdated: &t,
	}
}

func TestFindISCSIServerNodeName_ExplicitServer(t *testing.T) {
	statuses := []api.HostStatus{
		testHostStatus("hv1", "00000010", 5),
		testHostStatus("hv2", "00000020", 5),
	}
	statuses[1].IscsiServer = util.BoolPtr(true)

	node := findISCSIServerNodeName(statuses)
	if node != "hv2" {
		t.Fatalf("findISCSIServerNodeName() = %q, want %q", node, "hv2")
	}
}

func TestFindISCSIServerNodeName_FallbackByMinHostID(t *testing.T) {
	statuses := []api.HostStatus{
		testHostStatus("hv3", "00000030", 5),
		testHostStatus("hv1", "00000010", 5),
		testHostStatus("hv2", "00000020", 5),
	}

	node := findISCSIServerNodeName(statuses)
	if node != "hv1" {
		t.Fatalf("findISCSIServerNodeName() = %q, want %q", node, "hv1")
	}
}

func TestFindISCSIServerNodeName_IgnoreInactiveExplicitServer(t *testing.T) {
	statuses := []api.HostStatus{
		testHostStatus("hv1", "00000010", 60), // inactive
		testHostStatus("hv2", "00000005", 5),
		testHostStatus("hv3", "00000030", 5),
	}
	statuses[0].IscsiServer = util.BoolPtr(true)

	node := findISCSIServerNodeName(statuses)
	if node != "hv2" {
		t.Fatalf("findISCSIServerNodeName() = %q, want %q", node, "hv2")
	}
}

func TestFindISCSIServerNodeName_NoActiveHosts(t *testing.T) {
	statuses := []api.HostStatus{
		testHostStatus("hv1", "00000010", 120),
		testHostStatus("hv2", "00000020", 180),
	}

	node := findISCSIServerNodeName(statuses)
	if node != "" {
		t.Fatalf("findISCSIServerNodeName() = %q, want empty", node)
	}
}
