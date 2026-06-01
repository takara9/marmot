package networkfabric

import (
	"reflect"
	"strings"
	"testing"
)

func TestEnsureLoadBalancer_ProgramsOVNCommands(t *testing.T) {
	withOVNLookPath(t, true, true)
	calls := []ovnRunnerCall{}
	withOVNRunner(t, func(args ...string) (string, error) {
		calls = append(calls, ovnRunnerCall{args: append([]string{}, args...)})
		if len(args) >= 6 && reflect.DeepEqual(args[:5], []string{"--columns=_uuid", "--format=csv", "--data=bare", "--no-heading", "find"}) {
			return "\n", nil
		}
		return "", nil
	})

	of := NewOVNFabric()
	spec := OVNLoadBalancerSpec{
		LoadBalancerID:    "lb-1",
		LogicalSwitchName: "marmot-net-net1",
		Protocol:          "tcp",
		VIPs: map[string]string{
			"10.0.0.10:443": "10.0.0.21:443,10.0.0.22:443",
			"10.0.0.10:80":  "10.0.0.21:80,10.0.0.22:80",
		},
		ExternalIDs: map[string]string{
			"marmot_network_id": "net1",
		},
	}

	lbName, err := of.EnsureLoadBalancer(spec)
	if err != nil {
		t.Fatalf("EnsureLoadBalancer() failed: %v", err)
	}
	if lbName != "marmot-lb-lb-1" {
		t.Fatalf("EnsureLoadBalancer() name = %q, want %q", lbName, "marmot-lb-lb-1")
	}

	hasLBCreate := false
	hasProtocolSet := false
	hasClearVIPs := false
	hasAttach := false
	setVIPs := []string{}
	for _, c := range calls {
		if len(c.args) == 3 && reflect.DeepEqual(c.args[:2], []string{"create", "load_balancer"}) && c.args[2] == "name=marmot-lb-lb-1" {
			hasLBCreate = true
		}
		if len(c.args) == 4 && reflect.DeepEqual(c.args[:3], []string{"clear", "load_balancer", "marmot-lb-lb-1"}) && c.args[3] == "vips" {
			hasClearVIPs = true
		}
		if len(c.args) == 4 && reflect.DeepEqual(c.args[:3], []string{"set", "load_balancer", "marmot-lb-lb-1"}) && c.args[3] == "protocol=tcp" {
			hasProtocolSet = true
		}
		if len(c.args) == 4 && reflect.DeepEqual(c.args[:2], []string{"--may-exist", "ls-lb-add"}) {
			hasAttach = true
		}
		if len(c.args) == 4 && reflect.DeepEqual(c.args[:3], []string{"set", "load_balancer", "marmot-lb-lb-1"}) && strings.HasPrefix(c.args[3], "vips:") {
			setVIPs = append(setVIPs, c.args[3])
		}
	}

	if !hasLBCreate {
		t.Fatalf("load_balancer create was not called: calls=%v", calls)
	}
	if !hasProtocolSet {
		t.Fatalf("protocol set was not called: calls=%v", calls)
	}
	if !hasClearVIPs {
		t.Fatalf("clear vips was not called: calls=%v", calls)
	}
	if !hasAttach {
		t.Fatalf("ls-lb-add was not called: calls=%v", calls)
	}
	if len(setVIPs) != 2 {
		t.Fatalf("set vips call count = %d, want 2; calls=%v", len(setVIPs), setVIPs)
	}
	if !(strings.Contains(setVIPs[0], "10.0.0.10:443") || strings.Contains(setVIPs[1], "10.0.0.10:443")) {
		t.Fatalf("443 vip mapping not found: calls=%v", setVIPs)
	}
	if !(strings.Contains(setVIPs[0], "10.0.0.10:80") || strings.Contains(setVIPs[1], "10.0.0.10:80")) {
		t.Fatalf("80 vip mapping not found: calls=%v", setVIPs)
	}
}

func TestEnsureLoadBalancer_DoesNotCreateWhenNamedLoadBalancerExists(t *testing.T) {
	withOVNLookPath(t, true, true)
	calls := []ovnRunnerCall{}
	withOVNRunner(t, func(args ...string) (string, error) {
		calls = append(calls, ovnRunnerCall{args: append([]string{}, args...)})
		if len(args) >= 6 && reflect.DeepEqual(args[:5], []string{"--columns=_uuid", "--format=csv", "--data=bare", "--no-heading", "find"}) {
			return "a3f5f4f9-cf12-48c4-935a-6f5f5e6f4fd8\n", nil
		}
		return "", nil
	})

	of := NewOVNFabric()
	_, err := of.EnsureLoadBalancer(OVNLoadBalancerSpec{
		LoadBalancerID:    "lb-1",
		LogicalSwitchName: "marmot-net-net1",
		Protocol:          "tcp",
		VIPs: map[string]string{
			"10.0.0.10:80": "10.0.0.21:80",
		},
	})
	if err != nil {
		t.Fatalf("EnsureLoadBalancer() failed: %v", err)
	}

	for _, c := range calls {
		if len(c.args) >= 2 && reflect.DeepEqual(c.args[:2], []string{"create", "load_balancer"}) {
			t.Fatalf("unexpected create command when load balancer already exists: calls=%v", calls)
		}
	}
}

func TestDeleteLoadBalancer_CallsDetachAndDelete(t *testing.T) {
	withOVNLookPath(t, true, true)
	calls := []ovnRunnerCall{}
	withOVNRunner(t, func(args ...string) (string, error) {
		calls = append(calls, ovnRunnerCall{args: append([]string{}, args...)})
		if len(args) == 2 && reflect.DeepEqual(args[:2], []string{"lsp-list", "marmot-net-net1"}) {
			return "marmot-lb-vip-lb-1-aaaaaaaa\nother-port\n", nil
		}
		return "", nil
	})

	of := NewOVNFabric()
	if err := of.DeleteLoadBalancer("lb-1", "marmot-net-net1"); err != nil {
		t.Fatalf("DeleteLoadBalancer() failed: %v", err)
	}

	hasDetach := false
	hasListPorts := false
	hasDeleteVIPPort := false
	hasDeleteLB := false
	for _, c := range calls {
		if reflect.DeepEqual(c.args, []string{"--if-exists", "ls-lb-del", "marmot-net-net1", "marmot-lb-lb-1"}) {
			hasDetach = true
		}
		if reflect.DeepEqual(c.args, []string{"lsp-list", "marmot-net-net1"}) {
			hasListPorts = true
		}
		if reflect.DeepEqual(c.args, []string{"--if-exists", "lsp-del", "marmot-lb-vip-lb-1-aaaaaaaa"}) {
			hasDeleteVIPPort = true
		}
		if reflect.DeepEqual(c.args, []string{"--if-exists", "lb-del", "marmot-lb-lb-1"}) {
			hasDeleteLB = true
		}
	}
	if !hasDetach || !hasListPorts || !hasDeleteVIPPort || !hasDeleteLB {
		t.Fatalf("expected detach/list-vip-port/delete-vip-port/delete-lb calls, got=%v", calls)
	}
}

func TestGetLoadBalancerStatus_ExistsAndAttached(t *testing.T) {
	withOVNLookPath(t, true, true)
	withOVNRunner(t, func(args ...string) (string, error) {
		if len(args) >= 6 && reflect.DeepEqual(args[:5], []string{"--columns=_uuid", "--format=csv", "--data=bare", "--no-heading", "find"}) {
			return "a3f5f4f9-cf12-48c4-935a-6f5f5e6f4fd8\n", nil
		}
		if len(args) == 4 && reflect.DeepEqual(args[:3], []string{"get", "load_balancer", "marmot-lb-lb-1"}) {
			return "{\"10.0.0.10:80\"=\"10.0.0.21:80,10.0.0.22:80\"}", nil
		}
		if len(args) == 4 && reflect.DeepEqual(args[:3], []string{"get", "logical_switch", "marmot-net-net1"}) {
			return "[a3f5f4f9-cf12-48c4-935a-6f5f5e6f4fd8]", nil
		}
		return "", nil
	})

	of := NewOVNFabric()
	status, err := of.GetLoadBalancerStatus("lb-1", "marmot-net-net1")
	if err != nil {
		t.Fatalf("GetLoadBalancerStatus() failed: %v", err)
	}
	if !status.Exists {
		t.Fatalf("Exists = false, want true")
	}
	if !status.AttachedToLogicalSwitch {
		t.Fatalf("AttachedToLogicalSwitch = false, want true")
	}
	if status.VIPCount != 1 {
		t.Fatalf("VIPCount = %d, want 1", status.VIPCount)
	}
	if status.OVNLoadBalancerName != "marmot-lb-lb-1" {
		t.Fatalf("OVNLoadBalancerName = %q, want %q", status.OVNLoadBalancerName, "marmot-lb-lb-1")
	}
}

func TestGetLoadBalancerStatus_NotFound(t *testing.T) {
	withOVNLookPath(t, true, true)
	withOVNRunner(t, func(args ...string) (string, error) {
		if len(args) >= 6 && reflect.DeepEqual(args[:5], []string{"--columns=_uuid", "--format=csv", "--data=bare", "--no-heading", "find"}) {
			return "\n", nil
		}
		return "", nil
	})

	of := NewOVNFabric()
	status, err := of.GetLoadBalancerStatus("lb-1", "marmot-net-net1")
	if err != nil {
		t.Fatalf("GetLoadBalancerStatus() failed: %v", err)
	}
	if status.Exists {
		t.Fatalf("Exists = true, want false")
	}
	if status.AttachedToLogicalSwitch {
		t.Fatalf("AttachedToLogicalSwitch = true, want false")
	}
	if status.VIPCount != 0 {
		t.Fatalf("VIPCount = %d, want 0", status.VIPCount)
	}
}

func TestEnsureLoadBalancer_CleansStaleVIPPortWithSameIP(t *testing.T) {
	withOVNLookPath(t, true, true)
	calls := []ovnRunnerCall{}
	withOVNRunner(t, func(args ...string) (string, error) {
		calls = append(calls, ovnRunnerCall{args: append([]string{}, args...)})
		if len(args) >= 6 && reflect.DeepEqual(args[:5], []string{"--columns=_uuid", "--format=csv", "--data=bare", "--no-heading", "find"}) {
			return "\n", nil
		}
		if len(args) == 2 && reflect.DeepEqual(args[:2], []string{"lsp-list", "marmot-net-net1"}) {
			return "marmot-lb-vip-oldlb-aaaaaaaa\n", nil
		}
		if len(args) == 4 && reflect.DeepEqual(args[:3], []string{"get", "logical_switch_port", "marmot-lb-vip-oldlb-aaaaaaaa"}) && args[3] == "addresses" {
			return "[\"02:00:aa:aa:aa:aa 10.0.0.10\"]", nil
		}
		return "", nil
	})

	of := NewOVNFabric()
	_, err := of.EnsureLoadBalancer(OVNLoadBalancerSpec{
		LoadBalancerID:    "lb-1",
		LogicalSwitchName: "marmot-net-net1",
		Protocol:          "tcp",
		VIPs: map[string]string{
			"10.0.0.10:80": "10.0.0.21:80",
		},
	})
	if err != nil {
		t.Fatalf("EnsureLoadBalancer() failed: %v", err)
	}

	hasDeleteStale := false
	for _, c := range calls {
		if reflect.DeepEqual(c.args, []string{"--if-exists", "lsp-del", "marmot-lb-vip-oldlb-aaaaaaaa"}) {
			hasDeleteStale = true
			break
		}
	}
	if !hasDeleteStale {
		t.Fatalf("stale vip port delete was not called: calls=%v", calls)
	}
}

func TestParseOVNMap(t *testing.T) {
	raw := "{\"10.0.0.10:80\"=\"10.0.0.21:80,10.0.0.22:80\", \"10.0.0.10:443\"=\"10.0.0.21:443\"}"
	parsed := parseOVNMap(raw)
	if len(parsed) != 2 {
		t.Fatalf("parseOVNMap len = %d, want 2; got=%v", len(parsed), parsed)
	}
	if parsed["10.0.0.10:80"] != "10.0.0.21:80,10.0.0.22:80" {
		t.Fatalf("unexpected parsed value for 80: %v", parsed)
	}
}

func TestDuplicateVIPConflictPortName(t *testing.T) {
	err := 
		"ovn-nbctl failed: args=[lsp-set-addresses p1 02:00:aa:bb:cc:dd 172.16.80.5] output=" +
		"ovn-nbctl: Error on switch marmot-net-webs-net: duplicate IPv4 address '172.16.80.5' found on logical switch port 'marmot-lb-vip-old-12345678' err=exit status 1"
	port := duplicateVIPConflictPortName(assertErrString(err))
	if port != "marmot-lb-vip-old-12345678" {
		t.Fatalf("duplicateVIPConflictPortName() = %q, want %q", port, "marmot-lb-vip-old-12345678")
	}
}

func assertErrString(msg string) error {
	return &staticError{msg: msg}
}

type staticError struct {
	msg string
}

func (e *staticError) Error() string {
	return e.msg
}
