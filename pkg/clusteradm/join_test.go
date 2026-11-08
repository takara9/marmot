package clusteradm

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
)

const (
	testMasterIP   = "10.1.0.13"
	testClusterID  = "6f1c0c0e-test"
	testMasterURL  = "http://10.1.0.13:2379"
	testLocalURL   = "http://127.0.0.1:2379"
	testNodeName   = "hv3"
	testMasterNode = "hv1"
)

func strp(s string) *string { return &s }

type joinFixture struct {
	*fixture
	master *fakeStore
	// systemctl "start marmot.service" で、マスターへ自ノードが登録されたとみなす
	registerOnStart bool
}

func newJoinFixture(t *testing.T) *joinFixture {
	t.Helper()
	f := &joinFixture{fixture: newFixture(t)}
	f.registerOnStart = true
	f.master = &fakeStore{
		info:  &api.ClusterInfo{ClusterId: testClusterID, MasterNode: testMasterNode},
		hosts: []api.HostStatus{{NodeName: strp(testMasterNode), HostId: strp("11111111"), IpAddress: strp(testMasterIP)}},
	}
	f.env.OpenStore = func(url string) (ClusterStore, error) {
		switch url {
		case testMasterURL:
			return f.master, nil
		case testLocalURL:
			return f.store, nil
		}
		return nil, errors.New("unexpected url " + url)
	}
	f.env.Systemctl = func(args ...string) error {
		call := strings.Join(args, " ")
		f.calls = append(f.calls, call)
		if call == "start "+MarmotUnit && f.registerOnStart {
			f.master.hosts = append(f.master.hosts, api.HostStatus{NodeName: strp(testNodeName), HostId: strp("ad02fe16"), IpAddress: strp("10.1.0.14")})
		}
		return nil
	}
	return f
}

func (f *joinFixture) opts() JoinOptions {
	return JoinOptions{ClusterID: testClusterID, MasterNode: testMasterIP}
}

func TestJoinSuccess(t *testing.T) {
	f := newJoinFixture(t)
	var out bytes.Buffer
	if err := Join(context.Background(), f.env, f.opts(), &out); err != nil {
		t.Fatalf("Join() error = %v\n%s", err, out.String())
	}

	wantCalls := "stop marmot.service,disable --now etcd.service,start marmot.service"
	if strings.Join(f.calls, ",") != wantCalls {
		t.Fatalf("systemctl calls = %v, want %s", f.calls, wantCalls)
	}
	cfg := f.config(t)
	if !strings.Contains(cfg, `"etcd_url": "`+testMasterURL+`"`) || !strings.Contains(cfg, `"default_underlay_interface": "br0"`) {
		t.Fatalf("config = %s", cfg)
	}
	bak, _ := os.ReadFile(f.env.ConfigPath + ".bak")
	if string(bak) != testConfig {
		t.Fatalf(".bak differs from original")
	}
	for _, s := range []string{"CLUSTER-ID: " + testClusterID, testMasterNode, testNodeName} {
		if !strings.Contains(out.String(), s) {
			t.Fatalf("output missing %q:\n%s", s, out.String())
		}
	}
}

func TestJoinUnderlayInterfaceOption(t *testing.T) {
	f := newJoinFixture(t)
	f.env.InterfaceIPv4 = func(name string) (string, error) {
		if name == "enp2s0" {
			return "172.16.0.202", nil
		}
		return "", errors.New("no such interface")
	}
	o := f.opts()
	o.UnderlayInterface = "enp2s0"
	if err := Join(context.Background(), f.env, o, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.config(t), `"default_underlay_interface": "enp2s0"`) {
		t.Fatalf("config = %s", f.config(t))
	}
}

func TestJoinPrecheckErrorsChangeNothing(t *testing.T) {
	tests := []struct {
		name string
		mod  func(f *joinFixture, o *JoinOptions)
	}{
		{"not root", func(f *joinFixture, o *JoinOptions) { f.root = false }},
		{"master node with port", func(f *joinFixture, o *JoinOptions) { o.MasterNode = "10.1.0.13:2379" }},
		{"empty master node", func(f *joinFixture, o *JoinOptions) { o.MasterNode = "" }},
		{"unknown interface", func(f *joinFixture, o *JoinOptions) { o.UnderlayInterface = "nope0" }},
		{"hostid unavailable", func(f *joinFixture, o *JoinOptions) {
			f.env.HostID = func() (string, error) { return "", errors.New("no hostid") }
		}},
		{"already a member", func(f *joinFixture, o *JoinOptions) {
			f.store.info = &api.ClusterInfo{ClusterId: testClusterID}
		}},
		{"local managed objects exist", func(f *joinFixture, o *JoinOptions) {
			f.store.managed = []string{"/marmot/server/abc"}
		}},
		{"cluster id mismatch", func(f *joinFixture, o *JoinOptions) { o.ClusterID = "other-id" }},
		{"master cluster not enabled", func(f *joinFixture, o *JoinOptions) { f.master.info = nil }},
		{"master unreachable", func(f *joinFixture, o *JoinOptions) {
			orig := f.env.OpenStore
			f.env.OpenStore = func(url string) (ClusterStore, error) {
				if url == testMasterURL {
					return nil, errors.New("refused")
				}
				return orig(url)
			}
		}},
		{"duplicate node_name", func(f *joinFixture, o *JoinOptions) {
			f.master.hosts = append(f.master.hosts, api.HostStatus{NodeName: strp(testNodeName), HostId: strp("22222222")})
		}},
		{"duplicate hostid", func(f *joinFixture, o *JoinOptions) {
			f.master.hosts = append(f.master.hosts, api.HostStatus{NodeName: strp("hv9"), HostId: strp("ad02fe16")})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newJoinFixture(t)
			o := f.opts()
			tc.mod(f, &o)
			if err := Join(context.Background(), f.env, o, &bytes.Buffer{}); err == nil {
				t.Fatal("expected error")
			}
			if f.config(t) != testConfig || len(f.calls) != 0 {
				t.Fatalf("state changed: calls=%v", f.calls)
			}
			if _, err := os.Stat(f.env.ConfigPath + ".bak"); !os.IsNotExist(err) {
				t.Fatalf(".bak must not be created")
			}
		})
	}
}

func TestJoinDryRunChangesNothing(t *testing.T) {
	f := newJoinFixture(t)
	o := f.opts()
	o.DryRun = true
	if err := Join(context.Background(), f.env, o, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if f.config(t) != testConfig || len(f.calls) != 0 {
		t.Fatalf("dry-run changed state: calls=%v", f.calls)
	}
}

func TestJoinConfirmDeclined(t *testing.T) {
	f := newJoinFixture(t)
	o := f.opts()
	o.Confirm = func(string) bool { return false }
	if err := Join(context.Background(), f.env, o, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
	if f.config(t) != testConfig || len(f.calls) != 0 {
		t.Fatalf("state changed: calls=%v", f.calls)
	}
}

func TestJoinRollsBackWhenNotRegistered(t *testing.T) {
	f := newJoinFixture(t)
	f.registerOnStart = false
	err := Join(context.Background(), f.env, f.opts(), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if f.config(t) != testConfig {
		t.Fatalf("config not restored:\n%s", f.config(t))
	}
	wantCalls := "stop marmot.service,disable --now etcd.service,start marmot.service,stop marmot.service,enable --now etcd.service,restart marmot.service"
	if strings.Join(f.calls, ",") != wantCalls {
		t.Fatalf("systemctl calls = %v\nwant %s", f.calls, wantCalls)
	}
	if len(f.master.deletedHosts) != 1 || f.master.deletedHosts[0] != testNodeName {
		t.Fatalf("own host status must be deleted from master: %v", f.master.deletedHosts)
	}
}

func TestJoinRollsBackWhenEtcdDisableFails(t *testing.T) {
	f := newJoinFixture(t)
	f.env.Systemctl = func(args ...string) error {
		call := strings.Join(args, " ")
		f.calls = append(f.calls, call)
		if call == "disable --now "+EtcdUnit {
			return errors.New("disable failed")
		}
		return nil
	}
	if err := Join(context.Background(), f.env, f.opts(), &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
	if f.config(t) != testConfig {
		t.Fatalf("config must be unchanged")
	}
	// marmot は停止済みなので、元の稼働状態へ戻す
	if got := f.calls[len(f.calls)-1]; got != "restart "+MarmotUnit {
		t.Fatalf("last call = %q, calls=%v", got, f.calls)
	}
}

func TestJoinKeepsMarmotStoppedWhenItWasNotRunning(t *testing.T) {
	f := newJoinFixture(t)
	f.active[MarmotUnit] = false
	f.registerOnStart = false
	if err := Join(context.Background(), f.env, f.opts(), &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
	if got := f.calls[len(f.calls)-1]; strings.HasPrefix(got, "restart") {
		t.Fatalf("marmot must not be restarted when it was not running: %v", f.calls)
	}
}
