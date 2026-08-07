package controller

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestParsePortFromEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		wantPort int
		wantOk   bool
	}{
		{"http url", "http://127.0.0.1:2379", 2379, true},
		{"host port", "127.0.0.1:2380", 2380, true},
		{"empty", "", 0, false},
		{"no port", "127.0.0.1", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			port, ok := parsePortFromEndpoint(c.endpoint)
			if ok != c.wantOk || port != c.wantPort {
				t.Fatalf("parsePortFromEndpoint(%q) = (%d, %v), want (%d, %v)", c.endpoint, port, ok, c.wantPort, c.wantOk)
			}
		})
	}
}

func TestAllocateKubernetesEngineEtcdPortsAvoidsOwnEtcdAndOtherClusters(t *testing.T) {
	rangeStart, rangeEnd := 40000, 40010

	existing := api.KubernetesEngine{
		Status: &api.Status{
			EtcdClientPort: util.IntPtrInt(40000),
			EtcdPeerPort:   util.IntPtrInt(40001),
		},
	}

	clientPort, peerPort, err := AllocateKubernetesEngineEtcdPorts([]api.KubernetesEngine{existing}, "http://127.0.0.1:40002", rangeStart, rangeEnd)
	if err != nil {
		t.Fatalf("AllocateKubernetesEngineEtcdPorts() failed: %v", err)
	}
	if clientPort == 40000 || clientPort == 40001 || clientPort == 40002 || peerPort == 40002 {
		t.Fatalf("allocated ports (%d, %d) collide with reserved ports", clientPort, peerPort)
	}
	if peerPort != clientPort+1 {
		t.Fatalf("peerPort = %d, want clientPort+1 (%d)", peerPort, clientPort+1)
	}
}

func TestAllocateKubernetesEngineEtcdPortsSkipsPortsNotLocallyFree(t *testing.T) {
	rangeStart, rangeEnd := 41000, 41010

	origProbe := kubernetesEnginePortProbe
	t.Cleanup(func() { kubernetesEnginePortProbe = origProbe })
	kubernetesEnginePortProbe = func(port int) bool {
		// 最初の候補ペアだけ使用中(bind不可)を模擬する。
		return port != 41000 && port != 41001
	}

	clientPort, peerPort, err := AllocateKubernetesEngineEtcdPorts(nil, "", rangeStart, rangeEnd)
	if err != nil {
		t.Fatalf("AllocateKubernetesEngineEtcdPorts() failed: %v", err)
	}
	if clientPort != 41002 || peerPort != 41003 {
		t.Fatalf("allocated ports = (%d, %d), want (41002, 41003)", clientPort, peerPort)
	}
}

func TestAllocateKubernetesEngineEtcdPortsExhausted(t *testing.T) {
	origProbe := kubernetesEnginePortProbe
	t.Cleanup(func() { kubernetesEnginePortProbe = origProbe })
	kubernetesEnginePortProbe = func(int) bool { return false }

	if _, _, err := AllocateKubernetesEngineEtcdPorts(nil, "", 42000, 42004); err == nil {
		t.Fatalf("expected error when no free port pair is available")
	}
}
