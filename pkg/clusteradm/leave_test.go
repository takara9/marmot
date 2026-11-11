package clusteradm

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

const memberConfig = `{
  "node_name": "hv3",
  "etcd_url": "http://10.1.0.13:2379",
  "default_underlay_interface": "br0",
  "os_images": [{"name": "ubuntu22.04"}]
}`

type leaveFixture struct {
	*fixture
	master *fakeStore
	runs   []string
	// ovs-vsctl の get に対する応答
	ovnRemote string
	systemID  string
	runErr    map[string]error
}

func newLeaveFixture(t *testing.T) *leaveFixture {
	t.Helper()
	f := &leaveFixture{
		fixture:   newFixture(t),
		ovnRemote: `"tcp:10.1.0.13:6642"`,
		systemID:  `"sys-hv3"`,
		runErr:    map[string]error{},
	}
	if err := os.WriteFile(f.env.ConfigPath, []byte(memberConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	f.active[EtcdUnit] = false // メンバーのノードでは、ローカル etcd は停止している
	f.master = &fakeStore{
		info: &api.ClusterInfo{ClusterId: testClusterID, MasterNode: testMasterNode},
		hosts: []api.HostStatus{
			{NodeName: strp(testMasterNode)},
			{NodeName: strp(testNodeName)},
		},
	}
	f.env.OpenStore = func(url string) (ClusterStore, error) {
		if url == testMasterURL {
			return f.master, nil
		}
		return nil, errors.New("unexpected url " + url)
	}
	f.env.Run = func(name string, args ...string) (string, error) {
		cmd := name + " " + strings.Join(args, " ")
		f.runs = append(f.runs, cmd)
		if err, ok := f.runErr[name]; ok {
			return "", err
		}
		switch {
		case strings.Contains(cmd, "external_ids:ovn-remote"):
			return f.ovnRemote, nil
		case strings.Contains(cmd, "external_ids:system-id"):
			return f.systemID, nil
		}
		return "", nil
	}
	return f
}

func (f *leaveFixture) opts() LeaveOptions { return LeaveOptions{ClusterID: testClusterID} }

func TestLeaveWithoutServers(t *testing.T) {
	f := newLeaveFixture(t)
	var out bytes.Buffer
	if err := Leave(context.Background(), f.env, f.opts(), &out); err != nil {
		t.Fatalf("Leave() error = %v\n%s", err, out.String())
	}

	wantCalls := "stop marmot.service,stop marmot.service,enable --now etcd.service,start marmot.service"
	if strings.Join(f.calls, ",") != wantCalls {
		t.Fatalf("systemctl calls = %v, want %s", f.calls, wantCalls)
	}
	if !strings.Contains(f.config(t), `"etcd_url": "http://127.0.0.1:2379"`) || !strings.Contains(f.config(t), `"default_underlay_interface": "br0"`) {
		t.Fatalf("config = %s", f.config(t))
	}
	bak, _ := os.ReadFile(f.env.ConfigPath + ".bak")
	if string(bak) != memberConfig {
		t.Fatalf(".bak differs from original")
	}
	if len(f.master.deletedHosts) != 1 || f.master.deletedHosts[0] != testNodeName {
		t.Fatalf("host status deleted = %v", f.master.deletedHosts)
	}
	wantRuns := []string{
		"ovs-vsctl get open . external_ids:ovn-remote",
		"ovs-vsctl --if-exists del-port mgmt-host",
		"ovs-vsctl get open . external_ids:system-id",
		"ovs-vsctl remove open . external_ids ovn-remote",
		"ovn-sbctl --db=tcp:10.1.0.13:6642 chassis-del sys-hv3",
	}
	if strings.Join(f.runs, "|") != strings.Join(wantRuns, "|") {
		t.Fatalf("runs = %v\nwant %v", f.runs, wantRuns)
	}
	if len(f.master.handedOver) != 1 || f.master.handedOver[0] != testNodeName+"->"+testMasterNode {
		t.Fatalf("management head hand-over = %v", f.master.handedOver)
	}
	if len(f.master.deletedFollowers) != 1 || f.master.deletedFollowers[0] != testNodeName {
		t.Fatalf("follower networks deleted = %v", f.master.deletedFollowers)
	}
	if !strings.Contains(out.String(), "シングルノードとして起動しました") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestLeaveAbortsWhenManagementHeadHandOverFails(t *testing.T) {
	f := newLeaveFixture(t)
	f.master.handOverErr = errors.New("no follower")
	err := Leave(context.Background(), f.env, f.opts(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "head") {
		t.Fatalf("expected hand-over error, got %v", err)
	}
	if len(f.master.deletedFollowers) != 0 || len(f.master.deletedHosts) != 0 {
		t.Fatalf("must not delete cluster objects: followers=%v hosts=%v", f.master.deletedFollowers, f.master.deletedHosts)
	}
}

func TestLeaveDeletesServers(t *testing.T) {
	f := newLeaveFixture(t)
	f.master.servers = []ServerRef{
		{ID: "a1", Name: "vm-run", StatusCode: db.SERVER_RUNNING},
		{ID: "b2", Name: "vm-stop", StatusCode: db.SERVER_STOPPED},
	}
	var out bytes.Buffer
	if err := Leave(context.Background(), f.env, f.opts(), &out); err != nil {
		t.Fatalf("Leave() error = %v\n%s", err, out.String())
	}
	if strings.Join(f.master.stopped, ",") != "a1" {
		t.Fatalf("only running VMs must be stopped: %v", f.master.stopped)
	}
	if strings.Join(f.master.deletedSrv, ",") != "a1,b2" || len(f.master.servers) != 0 {
		t.Fatalf("deleted = %v, remaining = %v", f.master.deletedSrv, f.master.servers)
	}
}

func TestLeaveForceDeletesWhenShutdownTimesOut(t *testing.T) {
	f := newLeaveFixture(t)
	f.master.neverStop = true
	f.master.servers = []ServerRef{{ID: "a1", Name: "vm-run", StatusCode: db.SERVER_RUNNING}}
	var out bytes.Buffer
	if err := Leave(context.Background(), f.env, f.opts(), &out); err != nil {
		t.Fatalf("Leave() error = %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "強制停止") || len(f.master.servers) != 0 {
		t.Fatalf("output = %s, remaining = %v", out.String(), f.master.servers)
	}
}

func TestLeaveAbortsWhenServerDeletionFails(t *testing.T) {
	f := newLeaveFixture(t)
	f.master.stuckDelete = true
	f.master.servers = []ServerRef{{ID: "a1", Name: "vm-stop", StatusCode: db.SERVER_STOPPED}}
	if err := Leave(context.Background(), f.env, f.opts(), &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
	if f.config(t) != memberConfig || len(f.calls) != 0 || len(f.master.deletedHosts) != 0 {
		t.Fatalf("must stay a member: calls=%v hosts=%v", f.calls, f.master.deletedHosts)
	}
}

func TestLeaveAbortsWhenVolumeDeletionIsPending(t *testing.T) {
	f := newLeaveFixture(t)
	f.master.volumes = map[string]bool{"v1": true}
	f.master.servers = []ServerRef{{ID: "a1", Name: "vm-stop", StatusCode: db.SERVER_STOPPED, VolumeIDs: []string{"v1"}}}
	if err := Leave(context.Background(), f.env, f.opts(), &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
	if f.config(t) != memberConfig || len(f.calls) != 0 || len(f.master.deletedHosts) != 0 {
		t.Fatalf("must stay a member: calls=%v hosts=%v", f.calls, f.master.deletedHosts)
	}
}

func TestLeaveRerunKeepsWaitingForPendingVolumes(t *testing.T) {
	f := newLeaveFixture(t)
	f.master.volumes = map[string]bool{"v1": true}
	f.master.servers = []ServerRef{{ID: "a1", Name: "vm-stop", StatusCode: db.SERVER_STOPPED, VolumeIDs: []string{"v1"}}}
	if err := Leave(context.Background(), f.env, f.opts(), &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
	if len(f.master.servers) != 0 {
		t.Fatalf("servers must be gone: %v", f.master.servers)
	}
	if err := Leave(context.Background(), f.env, f.opts(), &bytes.Buffer{}); err == nil {
		t.Fatal("rerun must still wait for the pending volume")
	}
	if f.config(t) != memberConfig || len(f.master.deletedHosts) != 0 {
		t.Fatalf("must stay a member: hosts=%v", f.master.deletedHosts)
	}
	f.master.volumes = map[string]bool{}
	if err := Leave(context.Background(), f.env, f.opts(), &bytes.Buffer{}); err != nil {
		t.Fatalf("rerun after volumes are gone: %v", err)
	}
}

func TestLeavePrecheckErrorsChangeNothing(t *testing.T) {
	tests := []struct {
		name string
		mod  func(f *leaveFixture, o *LeaveOptions)
	}{
		{"not root", func(f *leaveFixture, o *LeaveOptions) { f.root = false }},
		{"cluster id mismatch", func(f *leaveFixture, o *LeaveOptions) { o.ClusterID = "other" }},
		{"not a member", func(f *leaveFixture, o *LeaveOptions) { f.master.info = nil }},
		{"this node is the master", func(f *leaveFixture, o *LeaveOptions) {
			f.master.info.MasterNode = testNodeName
		}},
		{"master unreachable", func(f *leaveFixture, o *LeaveOptions) {
			f.env.OpenStore = func(string) (ClusterStore, error) { return nil, errors.New("refused") }
		}},
		{"servers exist but marmot is stopped", func(f *leaveFixture, o *LeaveOptions) {
			f.master.servers = []ServerRef{{ID: "a1", Name: "vm", StatusCode: db.SERVER_RUNNING}}
			f.active[MarmotUnit] = false
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newLeaveFixture(t)
			o := f.opts()
			tc.mod(f, &o)
			if err := Leave(context.Background(), f.env, o, &bytes.Buffer{}); err == nil {
				t.Fatal("expected error")
			}
			if f.config(t) != memberConfig || len(f.calls) != 0 || len(f.master.deletedHosts) != 0 || len(f.master.deletedSrv) != 0 {
				t.Fatalf("state changed: calls=%v", f.calls)
			}
		})
	}
}

func TestLeaveDryRunAndConfirm(t *testing.T) {
	f := newLeaveFixture(t)
	f.master.servers = []ServerRef{{ID: "a1", Name: "vm-run", StatusCode: db.SERVER_RUNNING}}
	var out bytes.Buffer
	o := f.opts()
	o.DryRun = true
	if err := Leave(context.Background(), f.env, o, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "vm-run") {
		t.Fatalf("dry-run must list VMs to delete: %s", out.String())
	}
	o = f.opts()
	o.Confirm = func(string) bool { return false }
	if err := Leave(context.Background(), f.env, o, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error when declined")
	}
	if f.config(t) != memberConfig || len(f.calls) != 0 || len(f.master.deletedSrv) != 0 || len(f.master.stopped) != 0 {
		t.Fatalf("state changed: calls=%v", f.calls)
	}
}

func TestLeaveOVNFailureIsOnlyAWarning(t *testing.T) {
	f := newLeaveFixture(t)
	f.runErr["ovn-sbctl"] = errors.New("connection refused")
	var out bytes.Buffer
	if err := Leave(context.Background(), f.env, f.opts(), &out); err != nil {
		t.Fatalf("Leave() error = %v", err)
	}
	if !strings.Contains(out.String(), "警告") || !strings.Contains(out.String(), "chassis-del sys-hv3") {
		t.Fatalf("output = %s", out.String())
	}
	if f.calls[len(f.calls)-1] != "start marmot.service" {
		t.Fatalf("marmot must be started: %v", f.calls)
	}
}

func TestLeaveSkipsOVNWhenOvsIsMissing(t *testing.T) {
	f := newLeaveFixture(t)
	f.runErr["ovs-vsctl"] = exec.ErrNotFound
	if err := Leave(context.Background(), f.env, f.opts(), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(f.runs) != 1 {
		t.Fatalf("runs = %v", f.runs)
	}
}

func TestLeaveResumesAfterInterruptedRun(t *testing.T) {
	f := newLeaveFixture(t)
	// 前回: 設定を戻したが、ローカル etcd の起動で失敗した状態
	if err := os.WriteFile(f.env.ConfigPath+".bak", []byte(memberConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	local := strings.Replace(memberConfig, "http://10.1.0.13:2379", "http://127.0.0.1:2379", 1)
	if err := os.WriteFile(f.env.ConfigPath, []byte(local), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.env.ConfigPath+".leave", []byte(testClusterID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.env.OpenStore = func(string) (ClusterStore, error) {
		t.Fatal("resume must not connect to etcd")
		return nil, nil
	}
	var out bytes.Buffer
	if err := Leave(context.Background(), f.env, f.opts(), &out); err != nil {
		t.Fatalf("Leave() error = %v\n%s", err, out.String())
	}
	if strings.Join(f.calls, ",") != "stop marmot.service,enable --now etcd.service,start marmot.service" {
		t.Fatalf("calls = %v", f.calls)
	}
	bak, _ := os.ReadFile(f.env.ConfigPath + ".bak")
	if string(bak) != memberConfig {
		t.Fatalf(".bak must keep the original configuration")
	}
}

func TestLeaveDoesNotResumeOnPlainSingleNode(t *testing.T) {
	f := newLeaveFixture(t)
	if err := os.WriteFile(f.env.ConfigPath, []byte(testConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	f.env.OpenStore = func(string) (ClusterStore, error) { return &fakeStore{}, nil }
	err := Leave(context.Background(), f.env, f.opts(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "メンバーではありません") {
		t.Fatalf("err = %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("calls = %v", f.calls)
	}
}

func TestLeaveReportsIncompleteStep(t *testing.T) {
	f := newLeaveFixture(t)
	f.env.Systemctl = func(args ...string) error {
		call := strings.Join(args, " ")
		f.calls = append(f.calls, call)
		if call == "enable --now "+EtcdUnit {
			return errors.New("enable failed")
		}
		return nil
	}
	err := Leave(context.Background(), f.env, f.opts(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "手順4") {
		t.Fatalf("err = %v", err)
	}
	if len(f.master.deletedHosts) != 1 {
		t.Fatalf("step 3 must be done before step 4")
	}
}

func TestLeaveResumesBeforeConfigChange(t *testing.T) {
	f := newLeaveFixture(t)
	// 前回: HostStatus の削除後、設定の変更前に中断された状態
	if err := os.WriteFile(f.env.ConfigPath+".leave", []byte(testClusterID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Leave(context.Background(), f.env, f.opts(), &out); err != nil {
		t.Fatalf("Leave() error = %v\n%s", err, out.String())
	}
	if len(f.master.deletedHosts) != 1 {
		t.Fatalf("deletedHosts = %v", f.master.deletedHosts)
	}
	if _, err := os.Stat(f.env.ConfigPath + ".leave"); !os.IsNotExist(err) {
		t.Fatalf("state file must be removed: %v", err)
	}
}
