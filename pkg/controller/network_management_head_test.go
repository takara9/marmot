package controller

import (
	"reflect"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

func mgmtTestNetwork(id, node, role string) api.VirtualNetwork {
	labels := map[string]interface{}{}
	db.SetNetworkSyncLabels(labels, role, "", node)
	n := api.VirtualNetwork{Metadata: api.Metadata{Name: "mgmt", NodeName: util.StringPtr(node), Labels: &labels}}
	api.SetVirtualNetworkID(&n, id)
	return n
}

func TestManagementHeadToHandOver(t *testing.T) {
	statuses := []api.HostStatus{{NodeName: util.StringPtr("node-c")}, {NodeName: util.StringPtr("node-b")}}

	t.Run("head のノードがクラスタに居なければ引き継ぎ対象", func(t *testing.T) {
		vnets := []api.VirtualNetwork{
			mgmtTestNetwork("f1", "node-b", "follower"),
			mgmtTestNetwork("h1", "node-a", "head"),
		}
		id, members, ok := managementHeadToHandOver(vnets, statuses)
		if !ok || id != "h1" || !reflect.DeepEqual(members, []string{"node-b", "node-c"}) {
			t.Fatalf("got id=%q members=%v ok=%v", id, members, ok)
		}
	})

	t.Run("head のノードがメンバーなら対象外", func(t *testing.T) {
		vnets := []api.VirtualNetwork{mgmtTestNetwork("h1", "node-b", "head")}
		if _, _, ok := managementHeadToHandOver(vnets, statuses); ok {
			t.Fatal("must not hand over while the head node is a member")
		}
	})

	t.Run("mgmt 以外のネットワークと HostStatus が空の場合は対象外", func(t *testing.T) {
		other := mgmtTestNetwork("h2", "node-a", "head")
		other.Metadata.Name = "user-net"
		if _, _, ok := managementHeadToHandOver([]api.VirtualNetwork{other}, statuses); ok {
			t.Fatal("only mgmt must be handed over")
		}
		if _, _, ok := managementHeadToHandOver([]api.VirtualNetwork{mgmtTestNetwork("h1", "node-a", "head")}, nil); ok {
			t.Fatal("must not hand over without cluster members")
		}
	})
}
