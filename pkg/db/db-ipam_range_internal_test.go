package db

import (
	"net/netip"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestAllocationRange_DefaultIPv4(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.10.0/24")
	ipnet := &api.IPNetwork{}

	start, end, err := allocationRange(prefix, ipnet)
	if err != nil {
		t.Fatalf("allocationRange() error = %v", err)
	}
	if got, want := start.String(), "192.168.10.2"; got != want {
		t.Fatalf("start = %s, want %s", got, want)
	}
	if got, want := end.String(), "192.168.10.254"; got != want {
		t.Fatalf("end = %s, want %s", got, want)
	}
}

func TestAllocationRange_CustomRange(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.20.0/24")
	ipnet := &api.IPNetwork{
		StartAddress: util.StringPtr("192.168.20.129"),
		EndAddress:   util.StringPtr("192.168.20.190"),
	}

	start, end, err := allocationRange(prefix, ipnet)
	if err != nil {
		t.Fatalf("allocationRange() error = %v", err)
	}
	if got, want := start.String(), "192.168.20.129"; got != want {
		t.Fatalf("start = %s, want %s", got, want)
	}
	if got, want := end.String(), "192.168.20.190"; got != want {
		t.Fatalf("end = %s, want %s", got, want)
	}
}

func TestAllocationRange_InvalidOrder(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.30.0/24")
	ipnet := &api.IPNetwork{
		StartAddress: util.StringPtr("192.168.30.200"),
		EndAddress:   util.StringPtr("192.168.30.100"),
	}

	_, _, err := allocationRange(prefix, ipnet)
	if err == nil {
		t.Fatal("allocationRange() error = nil, want error")
	}
}

func TestParseAndValidateRangeAddress_OutsidePrefix(t *testing.T) {
	prefix := netip.MustParsePrefix("10.0.0.0/24")
	_, err := parseAndValidateRangeAddress("10.0.1.10", prefix, "startAddress")
	if err == nil {
		t.Fatal("parseAndValidateRangeAddress() error = nil, want error")
	}
}

func TestShouldSkipReservedIPv4Address(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.40.0/24")

	if !shouldSkipReservedIPv4Address(prefix, netip.MustParseAddr("192.168.40.0")) {
		t.Fatal("network address should be reserved")
	}
	if !shouldSkipReservedIPv4Address(prefix, netip.MustParseAddr("192.168.40.1")) {
		t.Fatal("gateway address should be reserved")
	}
	if !shouldSkipReservedIPv4Address(prefix, netip.MustParseAddr("192.168.40.255")) {
		t.Fatal("broadcast address should be reserved")
	}
	if shouldSkipReservedIPv4Address(prefix, netip.MustParseAddr("192.168.40.2")) {
		t.Fatal("regular host address should not be reserved")
	}
}
