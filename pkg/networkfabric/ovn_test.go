package networkfabric

import (
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
)

type ovnRunnerCall struct {
	args []string
}

func withOVNRunner(t *testing.T, fn func(args ...string) (string, error)) {
	t.Helper()
	orig := runOVNNBCTLCommand
	runOVNNBCTLCommand = fn
	t.Cleanup(func() {
		runOVNNBCTLCommand = orig
	})
}

func withOVNSBRunner(t *testing.T, fn func(args ...string) (string, error)) {
	t.Helper()
	orig := runOVNSBCTLCommand
	runOVNSBCTLCommand = fn
	t.Cleanup(func() {
		runOVNSBCTLCommand = orig
	})
}

func withOVSVSRunner(t *testing.T, fn func(args ...string) (string, error)) {
	t.Helper()
	orig := runOVSVSCTLCommand
	runOVSVSCTLCommand = fn
	t.Cleanup(func() {
		runOVSVSCTLCommand = orig
	})
}

func withOVNLookPath(t *testing.T, nbOK, sbOK bool) {
	t.Helper()
	origNB := ovnNBCTLLookPath
	origSB := ovnSBCTLLookPath
	ovnNBCTLLookPath = func(file string) (string, error) {
		if nbOK {
			return "/usr/bin/" + file, nil
		}
		return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
	}
	ovnSBCTLLookPath = func(file string) (string, error) {
		if sbOK {
			return "/usr/bin/" + file, nil
		}
		return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
	}
	t.Cleanup(func() {
		ovnNBCTLLookPath = origNB
		ovnSBCTLLookPath = origSB
	})
}

func withGeneveOVSTunnelMesh(t *testing.T, enabled bool) {
	t.Helper()
	orig := enableGeneveOVSTunnelMesh
	enableGeneveOVSTunnelMesh = enabled
	t.Cleanup(func() {
		enableGeneveOVSTunnelMesh = orig
	})
}

func testGeneveVNet() *api.VirtualNetwork {
	overlay := api.Geneve
	bridge := "br-test"
	return &api.VirtualNetwork{
		Metadata: api.Metadata{Id: "net-1", Name: "test/net one"},
		Spec: api.VirtualNetworkSpec{
			OverlayMode: &overlay,
			BridgeName:  &bridge,
		},
	}
}

func TestDesiredGenevePortNames_DedupAndSort(t *testing.T) {
	got := desiredGenevePortNames("marmot-net-test", []string{"10.0.0.2", "10.0.0.1", "10.0.0.2", "  ", "10.0.0.3"})
	if len(got) != 3 {
		t.Fatalf("unexpected port count: got=%d want=3", len(got))
	}
	if !sortStringsAreSorted(got) {
		t.Fatalf("expected sorted result, got=%v", got)
	}
}

func TestGenevePortNameForPeer_ContainsSwitchEntropy(t *testing.T) {
	p1 := genevePortNameForPeer("marmot-net-a", "10.0.0.1")
	p2 := genevePortNameForPeer("marmot-net-b", "10.0.0.1")
	if p1 == p2 {
		t.Fatalf("expected different port names across switches, got same: %s", p1)
	}
}

func TestPruneGeneveLogicalPorts_DeletesRemovedManagedPortsOnly(t *testing.T) {
	lsName := logicalSwitchName(testGeneveVNet())
	keepName := genevePortNameForPeer(lsName, "10.0.0.1")
	deleteName := genevePortNameForPeer(lsName, "10.0.0.2")
	calls := []ovnRunnerCall{}
	withOVNRunner(t, func(args ...string) (string, error) {
		calls = append(calls, ovnRunnerCall{args: append([]string{}, args...)})
		if len(args) >= 2 && args[0] == "lsp-list" {
			return strings.Join([]string{
				keepName,
				deleteName,
				"vm-port-1",
			}, "\n"), nil
		}
		return "", nil
	})

	vnet := testGeneveVNet()
	remain := []string{"10.0.0.1"}
	if err := pruneGeneveLogicalPorts(vnet, lsName, remain); err != nil {
		t.Fatalf("pruneGeneveLogicalPorts returned error: %v", err)
	}

	deleted := []string{}
	setPorts := []string{}
	for _, c := range calls {
		if len(c.args) >= 3 && c.args[0] == "--if-exists" && c.args[1] == "lsp-del" {
			deleted = append(deleted, c.args[2])
		}
		if len(c.args) >= 4 && c.args[0] == "set" && c.args[1] == "logical_switch_port" {
			setPorts = append(setPorts, c.args[2])
		}
	}

	if len(deleted) != 1 {
		t.Fatalf("unexpected deleted ports: %v", deleted)
	}
	if deleted[0] != deleteName {
		t.Fatalf("unexpected deleted port: %v", deleted[0])
	}
	if contains(setPorts, "vm-port-1") {
		t.Fatalf("non-managed port should not be updated: %v", setPorts)
	}
	if !contains(setPorts, keepName) {
		t.Fatalf("kept managed port should be updated: keep=%s calls=%v", keepName, setPorts)
	}
	if contains(setPorts, deleted[0]) {
		t.Fatalf("deleted port should not be updated: deleted=%s calls=%v", deleted[0], setPorts)
	}
}

func TestEnsureOverlayMesh_GeneveRequiresOVNCommands(t *testing.T) {
	withGeneveOVSTunnelMesh(t, false)
	withOVNLookPath(t, false, false)
	of := NewOVNFabric()
	vnet := testGeneveVNet()
	if err := of.EnsureOverlayMesh(vnet, []string{"10.0.0.1"}); err == nil {
		t.Fatalf("expected error when ovn commands are unavailable")
	}
}

func TestEnsureOverlayMesh_GeneveRequiresSouthboundEncap(t *testing.T) {
	withGeneveOVSTunnelMesh(t, false)
	withOVNLookPath(t, true, true)
	withOVNRunner(t, func(args ...string) (string, error) {
		return "", nil
	})
	withOVNSBRunner(t, func(args ...string) (string, error) {
		return "", nil
	})

	of := NewOVNFabric()
	vnet := testGeneveVNet()
	if err := of.EnsureOverlayMesh(vnet, []string{"10.0.0.2"}); err == nil {
		t.Fatalf("expected error when OVN southbound has no geneve encap")
	}
}

func TestEnsureOverlayMesh_GeneveSyncsLogicalSwitchAndPorts(t *testing.T) {
	withGeneveOVSTunnelMesh(t, false)
	withOVNLookPath(t, true, true)
	withOVSVSRunner(t, func(args ...string) (string, error) {
		if len(args) >= 4 && args[0] == "get" && args[3] == "external_ids:ovn-remote" {
			return "tcp:172.16.0.201:6642", nil
		}
		return "", nil
	})
	calls := []ovnRunnerCall{}
	withOVNSBRunner(t, func(args ...string) (string, error) {
		return "geneve", nil
	})
	withOVNRunner(t, func(args ...string) (string, error) {
		calls = append(calls, ovnRunnerCall{args: append([]string{}, args...)})
		if len(args) >= 2 && args[0] == "lsp-list" {
			return "", nil
		}
		return "", nil
	})

	of := NewOVNFabric()
	vnet := testGeneveVNet()
	peers := []string{"10.0.0.2", "10.0.0.1", "10.0.0.2"}
	if err := of.EnsureOverlayMesh(vnet, peers); err != nil {
		t.Fatalf("EnsureOverlayMesh returned error: %v", err)
	}

	hasLSAdd := false
	lspAdds := []string{}
	for _, c := range calls {
		if len(c.args) >= 3 && reflect.DeepEqual(c.args[:2], []string{"--may-exist", "ls-add"}) {
			hasLSAdd = true
		}
		if len(c.args) >= 4 && c.args[0] == "--may-exist" && c.args[1] == "lsp-add" {
			lspAdds = append(lspAdds, c.args[3])
		}
	}

	if !hasLSAdd {
		t.Fatalf("expected ls-add call, got calls=%v", calls)
	}
	if len(lspAdds) != 2 {
		t.Fatalf("expected deduped lsp-add count=2, got=%d calls=%v", len(lspAdds), lspAdds)
	}
}

func TestOVNDBTargetsFromOVSRemote(t *testing.T) {
	withOVSVSRunner(t, func(args ...string) (string, error) {
		if len(args) >= 4 && args[0] == "get" && args[3] == "external_ids:ovn-remote" {
			return "\"tcp:172.16.0.201:6642\"", nil
		}
		return "", nil
	})

	if got := ovnSouthboundDBTarget(); got != "tcp:172.16.0.201:6642" {
		t.Fatalf("unexpected southbound target: got=%q", got)
	}
	if got := ovnNorthboundDBTarget(); got != "tcp:172.16.0.201:6641" {
		t.Fatalf("unexpected northbound target: got=%q", got)
	}
}

func TestAppendOVNDBTargetArgs(t *testing.T) {
	got := appendOVNDBTargetArgs([]string{"list", "encap"}, "tcp:172.16.0.201:6642")
	want := []string{"--db=tcp:172.16.0.201:6642", "list", "encap"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args: got=%v want=%v", got, want)
	}
}

func sortStringsAreSorted(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i-1] > values[i] {
			return false
		}
	}
	return true
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func TestLogicalSwitchName_SanitizesInput(t *testing.T) {
	vnet := testGeneveVNet()
	got := logicalSwitchName(vnet)
	if got == "" {
		t.Fatalf("logical switch name should not be empty")
	}
	if strings.ContainsAny(got, " /.") {
		t.Fatalf("logical switch name should be sanitized: %s", got)
	}
	if !strings.HasPrefix(got, "marmot-net-") {
		t.Fatalf("unexpected prefix: %s", got)
	}
}

func TestListLogicalSwitchPorts_NotFoundAsEmpty(t *testing.T) {
	withOVNRunner(t, func(args ...string) (string, error) {
		return "", fmt.Errorf("ovn-nbctl failed: switch does not exist")
	})
	ports, err := listLogicalSwitchPorts("marmot-net-x")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if len(ports) != 0 {
		t.Fatalf("expected empty ports, got: %v", ports)
	}
}
