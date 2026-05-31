package controller

import (
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

func TestResolveSingleBackendIP(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{db: database, marmot: &marmotd.Marmot{NodeName: "hvc", Db: database}}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateInternalServer(t, database, "server-10", "web-servers", "172.16.10.2")

	ip, err := ctrl.resolveSingleBackendIP("server-10", "web-servers")
	if err != nil {
		t.Fatalf("resolveSingleBackendIP() failed: %v", err)
	}
	if ip != "172.16.10.2" {
		t.Fatalf("resolveSingleBackendIP() = %q, want %q", ip, "172.16.10.2")
	}
}

func TestResolveSingleBackendIP_MissingAddress(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{db: database, marmot: &marmotd.Marmot{NodeName: "hvc", Db: database}}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	_, err := database.MakeServerEntry(api.Server{
		ApiVersion: "v1",
		Kind:       "Server",
		Metadata: api.Metadata{
			Name:     "server-10",
			NodeName: util.StringPtr("hvc"),
		},
		Spec: api.ServerSpec{
			NetworkInterface: &[]api.NetworkInterface{{
				Networkname: "web-servers",
			}},
		},
	})
	if err != nil {
		t.Fatalf("MakeServerEntry() failed: %v", err)
	}

	_, err = ctrl.resolveSingleBackendIP("server-10", "web-servers")
	if err == nil {
		t.Fatalf("resolveSingleBackendIP() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "does not have an assigned IP address") {
		t.Fatalf("resolveSingleBackendIP() error = %q, want contains %q", err.Error(), "does not have an assigned IP address")
	}
}

func TestResolveAutoBackendsOnNetwork(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{db: database, marmot: &marmotd.Marmot{NodeName: "hvc", Db: database}}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")

	mkServer := func(name, ip string, status int, withLabel bool, netName string) {
		labels := map[string]interface{}{}
		if withLabel {
			labels["lb-enabled"] = "true"
		}
		server, err := database.MakeServerEntry(api.Server{
			ApiVersion: "v1",
			Kind:       "Server",
			Metadata: api.Metadata{
				Name:     name,
				NodeName: util.StringPtr("hvc"),
				Labels:   &labels,
			},
			Spec: api.ServerSpec{
				NetworkInterface: &[]api.NetworkInterface{{
					Networkname: netName,
					Address:     util.StringPtr(ip),
				}},
			},
		})
		if err != nil {
			t.Fatalf("MakeServerEntry() failed for %s: %v", name, err)
		}
		database.UpdateServerStatus(api.ServerID(server), status, "")
	}

	mkServer("web-2", "172.16.10.3", db.SERVER_RUNNING, true, "web-servers")
	mkServer("web-1", "172.16.10.2", db.SERVER_RUNNING, true, "web-servers")
	mkServer("web-down", "172.16.10.4", db.SERVER_STOPPED, true, "web-servers")
	mkServer("web-other-net", "172.17.10.5", db.SERVER_RUNNING, true, "db-net")
	mkServer("web-no-label", "172.16.10.6", db.SERVER_RUNNING, false, "web-servers")

	ips, err := ctrl.resolveAutoBackendsOnNetwork("web-servers", "lb-enabled", "true")
	if err != nil {
		t.Fatalf("resolveAutoBackendsOnNetwork() failed: %v", err)
	}
	want := []string{"172.16.10.2", "172.16.10.3"}
	if len(ips) != len(want) {
		t.Fatalf("resolveAutoBackendsOnNetwork() len = %d, want %d; got=%v", len(ips), len(want), ips)
	}
	for i := range ips {
		if ips[i] != want[i] {
			t.Fatalf("resolveAutoBackendsOnNetwork()[%d] = %q, want %q; got=%v", i, ips[i], want[i], ips)
		}
	}
}
