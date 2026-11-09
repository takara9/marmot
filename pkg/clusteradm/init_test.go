package clusteradm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

const testConfig = `{
  "node_name": "hv3",
  "etcd_url": "http://127.0.0.1:2379",
  "default_underlay_interface": "br0",
  "os_images": [{"name": "ubuntu22.04"}],
  "host-bridge-default": {"netmasklen": 24}
}`

type fakeStore struct {
	info      *api.ClusterInfo
	putErr    error
	deleted   bool
	getErr    error
	openCount int

	hosts        []api.HostStatus
	servers      []ServerRef
	stopped      []string
	deletedSrv   []string
	neverStop    bool
	stuckDelete  bool
	managed      []string
	deletedHosts []string
	volumes      map[string]bool
}

func (s *fakeStore) GetClusterInfo() (*api.ClusterInfo, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.info == nil {
		return nil, db.ErrNotFound
	}
	return s.info, nil
}
func (s *fakeStore) PutClusterInfo(i api.ClusterInfo) error {
	if s.putErr != nil {
		return s.putErr
	}
	s.info = &i
	return nil
}
func (s *fakeStore) DeleteClusterInfo() error { s.deleted = true; s.info = nil; return nil }
func (s *fakeStore) Close() error             { return nil }

func (s *fakeStore) ListHostStatus() ([]api.HostStatus, error) { return s.hosts, nil }
func (s *fakeStore) ListManagedObjects() ([]string, error)     { return s.managed, nil }
func (s *fakeStore) ListServersOnNode(string) ([]ServerRef, error) {
	return append([]ServerRef(nil), s.servers...), nil
}

func (s *fakeStore) StopServer(id string) error {
	s.stopped = append(s.stopped, id)
	if s.neverStop {
		return nil
	}
	for i := range s.servers {
		if s.servers[i].ID == id {
			s.servers[i].StatusCode = db.SERVER_STOPPED
		}
	}
	return nil
}

func (s *fakeStore) DeleteServer(id string) error {
	s.deletedSrv = append(s.deletedSrv, id)
	if s.stuckDelete {
		return nil
	}
	for i := range s.servers {
		if s.servers[i].ID == id {
			s.servers = append(s.servers[:i], s.servers[i+1:]...)
			break
		}
	}
	return nil
}

func (s *fakeStore) ExistingVolumeIDs(ids []string) ([]string, error) {
	var existing []string
	for _, id := range ids {
		if s.volumes[id] {
			existing = append(existing, id)
		}
	}
	return existing, nil
}

func (s *fakeStore) DeleteHostStatus(name string) error {
	s.deletedHosts = append(s.deletedHosts, name)
	return nil
}

type fixture struct {
	env       *Env
	store     *fakeStore
	dir       string
	calls     []string
	active    map[string]bool
	healthErr error
	root      bool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{
		dir:    dir,
		store:  &fakeStore{},
		active: map[string]bool{EtcdUnit: true, MarmotUnit: true},
		root:   true,
	}
	cfgPath := filepath.Join(dir, "marmotd.json")
	if err := os.WriteFile(cfgPath, []byte(testConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	f.env = &Env{
		ConfigPath: cfgPath,
		DropInPath: filepath.Join(dir, "etcd.service.d", "marmot-cluster.conf"),
		IsRoot:     func() bool { return f.root },
		InterfaceIPv4: func(name string) (string, error) {
			if name == "br0" {
				return "10.1.0.13", nil
			}
			return "", errors.New("no such interface")
		},
		Systemctl: func(args ...string) error {
			f.calls = append(f.calls, strings.Join(args, " "))
			return nil
		},
		UnitActive:      func(u string) bool { return f.active[u] },
		HostID:          func() (string, error) { return "ad02fe16", nil },
		EtcdHealth:      func(context.Context, string) error { return f.healthErr },
		OpenStore:       func(string) (ClusterStore, error) { f.store.openCount++; return f.store, nil },
		NewClusterID:    func() string { return "6f1c0c0e-test" },
		Now:             func() time.Time { return time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC) },
		WaitTimeout:     50 * time.Millisecond,
		ShutdownTimeout: 50 * time.Millisecond,
		DeleteTimeout:   50 * time.Millisecond,
		PollInterval:    10 * time.Millisecond,
		Run:             func(string, ...string) (string, error) { return "", nil },
	}
	return f
}

func (f *fixture) config(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(f.env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInitSuccess(t *testing.T) {
	f := newFixture(t)
	var out bytes.Buffer
	if err := Init(context.Background(), f.env, InitOptions{}, &out); err != nil {
		t.Fatalf("Init() error = %v\n%s", err, out.String())
	}

	if f.store.info == nil || f.store.info.ClusterId != "6f1c0c0e-test" || f.store.info.MasterNode != "hv3" {
		t.Fatalf("cluster info = %+v", f.store.info)
	}

	dropIn, err := os.ReadFile(f.env.DropInPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "--listen-client-urls http://127.0.0.1:2379,http://10.1.0.13:2379 --advertise-client-urls http://10.1.0.13:2379"
	if !strings.Contains(string(dropIn), want) {
		t.Fatalf("drop-in = %q, want to contain %q", dropIn, want)
	}

	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(f.config(t)), &cfg); err != nil {
		t.Fatal(err)
	}
	if string(cfg["etcd_url"]) != `"http://10.1.0.13:2379"` || string(cfg["default_underlay_interface"]) != `"br0"` {
		t.Fatalf("config = %s", f.config(t))
	}
	if _, err := os.Stat(f.env.ConfigPath + ".bak"); err != nil {
		t.Fatalf("marmotd.json.bak not created: %v", err)
	}
	bak, _ := os.ReadFile(f.env.ConfigPath + ".bak")
	if string(bak) != testConfig {
		t.Fatalf(".bak differs from original")
	}

	wantCalls := []string{"daemon-reload", "restart etcd.service", "restart marmot.service"}
	if strings.Join(f.calls, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("systemctl calls = %v, want %v", f.calls, wantCalls)
	}
	for _, s := range []string{"CLUSTER-ID: 6f1c0c0e-test", "maadm cluster join 6f1c0c0e-test --master-node 10.1.0.13"} {
		if !strings.Contains(out.String(), s) {
			t.Fatalf("output missing %q:\n%s", s, out.String())
		}
	}
}

func TestInitUsesExplicitEtcdIPAndInterface(t *testing.T) {
	f := newFixture(t)
	f.env.InterfaceIPv4 = func(name string) (string, error) {
		if name == "enp2s0" {
			return "172.16.0.201", nil
		}
		return "", errors.New("no such interface")
	}
	var out bytes.Buffer
	if err := Init(context.Background(), f.env, InitOptions{EtcdIP: "192.168.1.5", UnderlayInterface: "enp2s0"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.config(t), `"etcd_url": "http://192.168.1.5:2379"`) || !strings.Contains(f.config(t), `"default_underlay_interface": "enp2s0"`) {
		t.Fatalf("config = %s", f.config(t))
	}
}

func TestInitPreservesOtherConfigItems(t *testing.T) {
	f := newFixture(t)
	if err := Init(context.Background(), f.env, InitOptions{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var got, orig map[string]any
	if err := json.Unmarshal([]byte(f.config(t)), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(testConfig), &orig); err != nil {
		t.Fatal(err)
	}
	for k, v := range orig {
		if k == "etcd_url" || k == "default_underlay_interface" {
			continue
		}
		gb, _ := json.Marshal(got[k])
		ob, _ := json.Marshal(v)
		if string(gb) != string(ob) {
			t.Errorf("key %q changed: %s -> %s", k, ob, gb)
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(f.config(t)), "{\n  \"node_name\"") {
		t.Fatalf("key order not preserved:\n%s", f.config(t))
	}
}

func TestInitAlreadyEnabledIsIdempotent(t *testing.T) {
	f := newFixture(t)
	f.store.info = &api.ClusterInfo{ClusterId: "existing-id", MasterNode: "hv3"}
	var out bytes.Buffer
	if err := Init(context.Background(), f.env, InitOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "existing-id") {
		t.Fatalf("output = %q", out.String())
	}
	if f.config(t) != testConfig || len(f.calls) != 0 {
		t.Fatalf("must not change anything: calls=%v", f.calls)
	}
	if _, err := os.Stat(f.env.DropInPath); !os.IsNotExist(err) {
		t.Fatalf("drop-in must not be created")
	}
}

func TestInitDryRunChangesNothing(t *testing.T) {
	f := newFixture(t)
	var out bytes.Buffer
	if err := Init(context.Background(), f.env, InitOptions{DryRun: true}, &out); err != nil {
		t.Fatal(err)
	}
	if f.config(t) != testConfig || len(f.calls) != 0 || f.store.info != nil {
		t.Fatalf("dry-run changed state: calls=%v info=%v", f.calls, f.store.info)
	}
	if _, err := os.Stat(f.env.DropInPath); !os.IsNotExist(err) {
		t.Fatalf("drop-in must not be created")
	}
}

func TestInitConfirmDeclined(t *testing.T) {
	f := newFixture(t)
	err := Init(context.Background(), f.env, InitOptions{Confirm: func(string) bool { return false }}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if f.config(t) != testConfig || len(f.calls) != 0 {
		t.Fatalf("state changed: calls=%v", f.calls)
	}
}

func TestInitPrecheckErrorsChangeNothing(t *testing.T) {
	tests := []struct {
		name string
		mod  func(f *fixture)
		opts InitOptions
	}{
		{"not root", func(f *fixture) { f.root = false }, InitOptions{}},
		{"etcd stopped", func(f *fixture) { f.active[EtcdUnit] = false }, InitOptions{}},
		{"marmot stopped", func(f *fixture) { f.active[MarmotUnit] = false }, InitOptions{}},
		{"unknown interface", func(f *fixture) {}, InitOptions{UnderlayInterface: "nope0"}},
		{"invalid etcd ip", func(f *fixture) {}, InitOptions{EtcdIP: "not-an-ip"}},
		{"ipv6 etcd ip", func(f *fixture) {}, InitOptions{EtcdIP: "fe80::1"}},
		{"etcd unreachable", func(f *fixture) {
			f.env.OpenStore = func(string) (ClusterStore, error) { return nil, errors.New("refused") }
		}, InitOptions{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.mod(f)
			if err := Init(context.Background(), f.env, tc.opts, &bytes.Buffer{}); err == nil {
				t.Fatal("expected error")
			}
			if f.config(t) != testConfig || len(f.calls) != 0 {
				t.Fatalf("state changed: calls=%v", f.calls)
			}
			if _, err := os.Stat(f.env.DropInPath); !os.IsNotExist(err) {
				t.Fatalf("drop-in must not be created")
			}
		})
	}
}

func TestInitRollsBackOnEtcdHealthFailure(t *testing.T) {
	f := newFixture(t)
	f.healthErr = errors.New("unhealthy")
	if err := Init(context.Background(), f.env, InitOptions{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
	if f.config(t) != testConfig {
		t.Fatalf("config must be unchanged")
	}
	if _, err := os.Stat(f.env.DropInPath); !os.IsNotExist(err) {
		t.Fatalf("drop-in must be removed on rollback")
	}
	if f.store.info != nil {
		t.Fatalf("cluster info must not be stored")
	}
	// etcd のみ再起動し、marmot には触れない
	if strings.Contains(strings.Join(f.calls, ","), MarmotUnit) {
		t.Fatalf("marmot must not be restarted: %v", f.calls)
	}
}

func TestInitRollsBackOnClusterInfoSaveFailure(t *testing.T) {
	f := newFixture(t)
	f.store.putErr = errors.New("put failed")
	if err := Init(context.Background(), f.env, InitOptions{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
	if f.config(t) != testConfig {
		t.Fatalf("config not restored:\n%s", f.config(t))
	}
	if _, err := os.Stat(f.env.DropInPath); !os.IsNotExist(err) {
		t.Fatalf("drop-in must be removed on rollback")
	}
	wantTail := "restart etcd.service,restart marmot.service"
	if !strings.HasSuffix(strings.Join(f.calls, ","), wantTail) {
		t.Fatalf("rollback calls = %v", f.calls)
	}
}

func TestInitRollsBackOnMarmotStartFailure(t *testing.T) {
	f := newFixture(t)
	f.env.Systemctl = func(args ...string) error {
		f.calls = append(f.calls, strings.Join(args, " "))
		if len(f.calls) == 3 && args[1] == MarmotUnit {
			return errors.New("start failed")
		}
		return nil
	}
	if err := Init(context.Background(), f.env, InitOptions{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
	if f.config(t) != testConfig {
		t.Fatalf("config not restored")
	}
	if !f.store.deleted || f.store.info != nil {
		t.Fatalf("cluster info must be deleted on rollback")
	}
}

func TestInitRestoresExistingDropIn(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Dir(f.env.DropInPath), 0o755); err != nil {
		t.Fatal(err)
	}
	const orig = "[Service]\nEnvironment=\"DAEMON_ARGS=--name old\"\n"
	if err := os.WriteFile(f.env.DropInPath, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	f.store.putErr = errors.New("put failed")
	if err := Init(context.Background(), f.env, InitOptions{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
	got, _ := os.ReadFile(f.env.DropInPath)
	if string(got) != orig {
		t.Fatalf("drop-in = %q, want original", got)
	}
}

func TestUpdateJSONStringsAddsMissingKey(t *testing.T) {
	out, err := updateJSONStrings([]byte(`{"a":1}`), map[string]string{"b": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"a": 1`) || !strings.Contains(string(out), `"b": "x"`) {
		t.Fatalf("out = %s", out)
	}
}

func TestInitDropInUnsetsConflictingEtcdEnvironment(t *testing.T) {
	f := newFixture(t)
	if err := Init(context.Background(), f.env, InitOptions{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	dropIn, err := os.ReadFile(f.env.DropInPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "UnsetEnvironment=ETCD_LISTEN_CLIENT_URLS ETCD_ADVERTISE_CLIENT_URLS"
	if !strings.Contains(string(dropIn), want) {
		t.Fatalf("drop-in = %q, want to contain %q", dropIn, want)
	}
}

func TestInitWarnsAboutConflictingEtcdEnvironment(t *testing.T) {
	f := newFixture(t)
	f.env.EtcdEnvironment = func() (string, error) {
		return "DAEMON_ARGS= ETCD_NAME=hv1 ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:2379 ETCD_ADVERTISE_CLIENT_URLS=http://0.0.0.0:2379", nil
	}
	var out bytes.Buffer
	if err := Init(context.Background(), f.env, InitOptions{DryRun: true}, &out); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:2379", "ETCD_ADVERTISE_CLIENT_URLS=http://0.0.0.0:2379", "UnsetEnvironment"} {
		if !strings.Contains(out.String(), s) {
			t.Fatalf("output missing %q:\n%s", s, out.String())
		}
	}
	if strings.Contains(out.String(), "ETCD_NAME") {
		t.Fatalf("unrelated variable must not be reported:\n%s", out.String())
	}
}

func TestInitDoesNotWarnWithoutConflictingEtcdEnvironment(t *testing.T) {
	f := newFixture(t)
	f.env.EtcdEnvironment = func() (string, error) { return "DAEMON_ARGS= ETCD_NAME=hv1", nil }
	var out bytes.Buffer
	if err := Init(context.Background(), f.env, InitOptions{DryRun: true}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "注意") {
		t.Fatalf("unexpected warning:\n%s", out.String())
	}
}

func TestInitIgnoresEtcdEnvironmentLookupFailure(t *testing.T) {
	f := newFixture(t)
	f.env.EtcdEnvironment = func() (string, error) { return "", errors.New("systemctl failed") }
	if err := Init(context.Background(), f.env, InitOptions{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
}
