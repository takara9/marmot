package marmotd

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestResolveAndAssignServerNodeByStorage_LeavesUnassignedForScheduler(t *testing.T) {
	m := &Marmot{Db: newGatewayKeySyncTestDatabase(t), NodeName: "hv-local"}

	t.Run("nodeName未指定かつストレージ制約なしの場合は割り当てない", func(t *testing.T) {
		vm, err := m.Db.MakeServerEntry(api.Server{Metadata: api.Metadata{Name: "sched-unassigned"}})
		if err != nil {
			t.Fatalf("MakeServerEntry() failed: %v", err)
		}
		node, err := m.ResolveAndAssignServerNodeByStorage(api.ServerID(vm))
		if err != nil {
			t.Fatalf("ResolveAndAssignServerNodeByStorage() failed: %v", err)
		}
		if node != "" {
			t.Fatalf("node = %q, want empty", node)
		}
		got, err := m.Db.GetServerById(api.ServerID(vm))
		if err != nil {
			t.Fatalf("GetServerById() failed: %v", err)
		}
		if got.Metadata.NodeName != nil && *got.Metadata.NodeName != "" {
			t.Fatalf("metadata.nodeName = %q, want unset", *got.Metadata.NodeName)
		}
	})

	t.Run("指定済みのnodeNameは維持する", func(t *testing.T) {
		vm, err := m.Db.MakeServerEntry(api.Server{Metadata: api.Metadata{Name: "sched-assigned", NodeName: util.StringPtr("hv-other")}})
		if err != nil {
			t.Fatalf("MakeServerEntry() failed: %v", err)
		}
		node, err := m.ResolveAndAssignServerNodeByStorage(api.ServerID(vm))
		if err != nil {
			t.Fatalf("ResolveAndAssignServerNodeByStorage() failed: %v", err)
		}
		if node != "hv-other" {
			t.Fatalf("node = %q, want %q", node, "hv-other")
		}
	})
}
