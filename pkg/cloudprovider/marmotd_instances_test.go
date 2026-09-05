package cloudprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/takara9/marmot/pkg/client"
)

func newTestMarmotdEndpoint(t *testing.T, body string) *client.MarmotEndpoint {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)

	endpoint, err := client.NewMarmotdEp("http", ts.URL[len("http://"):], "/api/v1", 5, false)
	if err != nil {
		t.Fatalf("NewMarmotdEp() failed: %v", err)
	}
	return endpoint
}

const testServersJSON = `[
  {
    "apiVersion": "v1",
    "kind": "Server",
    "metadata": {
      "id": "srv-1",
      "name": "test-cluster-1-node-1",
      "labels": {"kubernetesEngineId": "ke-1", "kubernetesEngineRole": "node"}
    },
    "spec": {
      "cpu": 4,
      "memory": 8192,
      "networkInterface": [
        {"networkid": "n1", "networkname": "mke-test-cluster-1", "address": "172.16.1.10"},
        {"networkid": "n2", "networkname": "host-bridge", "address": "192.168.1.50"}
      ]
    },
    "status": {"statusCode": 3}
  },
  {
    "apiVersion": "v1",
    "kind": "Server",
    "metadata": {
      "id": "srv-2",
      "name": "other-cluster-node-1",
      "labels": {"kubernetesEngineId": "ke-2", "kubernetesEngineRole": "node"}
    },
    "spec": {"networkInterface": []},
    "status": {"statusCode": 2}
  }
]`

func TestMarmotdInstancesInstanceExists(t *testing.T) {
	endpoint := newTestMarmotdEndpoint(t, testServersJSON)
	instances := NewMarmotdInstances(endpoint, "ke-1", "mke-test-cluster-1", "host-bridge", "", "")

	exists, err := instances.InstanceExists(context.Background(), "test-cluster-1-node-1")
	if err != nil {
		t.Fatalf("InstanceExists() failed: %v", err)
	}
	if !exists {
		t.Fatalf("InstanceExists() = false, want true")
	}

	exists, err = instances.InstanceExists(context.Background(), "no-such-node")
	if err != nil {
		t.Fatalf("InstanceExists() failed: %v", err)
	}
	if exists {
		t.Fatalf("InstanceExists() = true, want false")
	}

	// 他クラスタが所有するノード名は、たとえ存在しても対象外(false)とする。
	exists, err = instances.InstanceExists(context.Background(), "other-cluster-node-1")
	if err != nil {
		t.Fatalf("InstanceExists() failed: %v", err)
	}
	if exists {
		t.Fatalf("InstanceExists() for other cluster's node = true, want false")
	}
}

func TestMarmotdInstancesInstanceShutdown(t *testing.T) {
	endpoint := newTestMarmotdEndpoint(t, testServersJSON)
	instances := NewMarmotdInstances(endpoint, "ke-1", "mke-test-cluster-1", "host-bridge", "", "")

	shutdown, err := instances.InstanceShutdown(context.Background(), "test-cluster-1-node-1")
	if err != nil {
		t.Fatalf("InstanceShutdown() failed: %v", err)
	}
	if !shutdown {
		t.Fatalf("InstanceShutdown() = false, want true (statusCode=3/STOPPED)")
	}

	if _, err := instances.InstanceShutdown(context.Background(), "no-such-node"); err == nil {
		t.Fatalf("InstanceShutdown() expected error for missing node, got nil")
	}
}

func TestMarmotdInstancesInstanceMetadata(t *testing.T) {
	endpoint := newTestMarmotdEndpoint(t, testServersJSON)
	instances := NewMarmotdInstances(endpoint, "ke-1", "mke-test-cluster-1", "host-bridge", "", "")

	meta, err := instances.InstanceMetadata(context.Background(), "test-cluster-1-node-1")
	if err != nil {
		t.Fatalf("InstanceMetadata() failed: %v", err)
	}
	if want := "marmot://ke-1/srv-1"; meta.ProviderID != want {
		t.Fatalf("ProviderID = %q, want %q", meta.ProviderID, want)
	}
	if want := "4vcpu-8192mi"; meta.InstanceType != want {
		t.Fatalf("InstanceType = %q, want %q", meta.InstanceType, want)
	}

	byType := make(map[NodeAddressType]string)
	for _, addr := range meta.NodeAddresses {
		byType[addr.Type] = addr.Address
	}
	if got := byType[NodeHostName]; got != "test-cluster-1-node-1" {
		t.Fatalf("Hostname address = %q, want %q", got, "test-cluster-1-node-1")
	}
	if got := byType[NodeInternalIP]; got != "172.16.1.10" {
		t.Fatalf("InternalIP address = %q, want %q", got, "172.16.1.10")
	}
	if got := byType[NodeExternalIP]; got != "192.168.1.50" {
		t.Fatalf("ExternalIP address = %q, want %q", got, "192.168.1.50")
	}

	if _, err := instances.InstanceMetadata(context.Background(), "no-such-node"); err == nil {
		t.Fatalf("InstanceMetadata() expected error for missing node, got nil")
	}
}

func TestMarmotdInstancesInstanceMetadataWithoutExternalNetwork(t *testing.T) {
	endpoint := newTestMarmotdEndpoint(t, testServersJSON)
	instances := NewMarmotdInstances(endpoint, "ke-1", "mke-test-cluster-1", "", "", "")

	meta, err := instances.InstanceMetadata(context.Background(), "test-cluster-1-node-1")
	if err != nil {
		t.Fatalf("InstanceMetadata() failed: %v", err)
	}
	for _, addr := range meta.NodeAddresses {
		if addr.Type == NodeExternalIP {
			t.Fatalf("unexpected ExternalIP address present when externalNetworkName is empty: %+v", meta.NodeAddresses)
		}
	}
}

func TestMarmotdInstancesInstanceMetadataRegionZone(t *testing.T) {
	endpoint := newTestMarmotdEndpoint(t, testServersJSON)
	instances := NewMarmotdInstances(endpoint, "ke-1", "mke-test-cluster-1", "host-bridge", "region-a", "zone-a")

	meta, err := instances.InstanceMetadata(context.Background(), "test-cluster-1-node-1")
	if err != nil {
		t.Fatalf("InstanceMetadata() failed: %v", err)
	}
	if meta.Region != "region-a" {
		t.Errorf("Region = %q, want %q", meta.Region, "region-a")
	}
	if meta.Zone != "zone-a" {
		t.Errorf("Zone = %q, want %q", meta.Zone, "zone-a")
	}
}
