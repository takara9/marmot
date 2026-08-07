package controller

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSystemdRecorder は systemctl 呼び出しの記録用ヘルパー。
type fakeSystemdRecorder struct {
	calls []string
}

func (f *fakeSystemdRecorder) record(action string) error {
	f.calls = append(f.calls, action)
	return nil
}

func installFakeSystemd(t *testing.T) *fakeSystemdRecorder {
	t.Helper()
	rec := &fakeSystemdRecorder{}

	origReload := systemdDaemonReload
	origEnable := systemdEnableUnit
	origDisable := systemdDisableUnit
	origStart := systemdStartUnit
	origStop := systemdStopUnit
	origUnitDir := etcdSystemdUnitDir

	systemdDaemonReload = func() error { return rec.record("daemon-reload") }
	systemdEnableUnit = func(unit string) error { return rec.record("enable:" + unit) }
	systemdDisableUnit = func(unit string) error { return rec.record("disable:" + unit) }
	systemdStartUnit = func(unit string) error { return rec.record("start:" + unit) }
	systemdStopUnit = func(unit string) error { return rec.record("stop:" + unit) }
	etcdSystemdUnitDir = t.TempDir()

	t.Cleanup(func() {
		systemdDaemonReload = origReload
		systemdEnableUnit = origEnable
		systemdDisableUnit = origDisable
		systemdStartUnit = origStart
		systemdStopUnit = origStop
		etcdSystemdUnitDir = origUnitDir
	})
	return rec
}

func TestKubernetesEngineEtcdUnitName(t *testing.T) {
	if got, want := KubernetesEngineEtcdUnitName("demo"), "mke-etcd-demo.service"; got != want {
		t.Fatalf("KubernetesEngineEtcdUnitName() = %q, want %q", got, want)
	}
}

// クラスタ専用etcdユニットの「作成→起動→停止→削除」の一連のライフサイクルを検証する。
func TestKubernetesEngineEtcdUnitLifecycle(t *testing.T) {
	rec := installFakeSystemd(t)
	dataDir := filepath.Join(t.TempDir(), "data")

	cfg := KubernetesEngineEtcdUnitConfig{
		ClusterName:    "demo",
		EtcdBinaryPath: "/var/lib/marmot/mke/etcd/v3.6.8-linux-amd64/etcd",
		DataDir:        dataDir,
		ClientPort:     23790,
		PeerPort:       23791,
	}

	// 作成→起動
	if err := CreateKubernetesEngineEtcdUnit(cfg); err != nil {
		t.Fatalf("CreateKubernetesEngineEtcdUnit() failed: %v", err)
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("data dir was not created: %v", err)
	}
	unitPath := kubernetesEngineEtcdUnitPath("demo")
	content, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("unit file was not written: %v", err)
	}
	if !strings.Contains(string(content), cfg.EtcdBinaryPath) {
		t.Fatalf("unit file does not reference etcd binary path: %s", content)
	}
	if !strings.Contains(string(content), "23790") || !strings.Contains(string(content), "23791") {
		t.Fatalf("unit file does not reference allocated ports: %s", content)
	}

	wantAfterCreate := []string{"daemon-reload", "enable:mke-etcd-demo.service", "start:mke-etcd-demo.service"}
	assertCallsEqual(t, rec.calls, wantAfterCreate)

	// 停止
	rec.calls = nil
	if err := StopKubernetesEngineEtcdUnit("demo"); err != nil {
		t.Fatalf("StopKubernetesEngineEtcdUnit() failed: %v", err)
	}
	assertCallsEqual(t, rec.calls, []string{"stop:mke-etcd-demo.service"})
	if _, err := os.Stat(unitPath); err != nil {
		t.Fatalf("unit file should remain after stop: %v", err)
	}

	// 削除
	rec.calls = nil
	if err := DeleteKubernetesEngineEtcdUnit("demo"); err != nil {
		t.Fatalf("DeleteKubernetesEngineEtcdUnit() failed: %v", err)
	}
	wantAfterDelete := []string{"stop:mke-etcd-demo.service", "disable:mke-etcd-demo.service", "daemon-reload"}
	assertCallsEqual(t, rec.calls, wantAfterDelete)
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Fatalf("unit file should be removed after delete, stat err = %v", err)
	}

	// 削除後の再削除は冪等(ユニットファイル不在でもエラーにならない)であること
	rec.calls = nil
	if err := DeleteKubernetesEngineEtcdUnit("demo"); err != nil {
		t.Fatalf("DeleteKubernetesEngineEtcdUnit() second call failed: %v", err)
	}
}

// DeleteKubernetesEngineEtcdUnit は「ユニット不在」相当の失敗のみ無視し、
// それ以外の失敗(権限不足等)はユニットファイルが既に存在しない場合でも
// 呼び出し元に伝播することを検証する。
func TestDeleteKubernetesEngineEtcdUnitPropagatesNonMissingErrors(t *testing.T) {
	installFakeSystemd(t)

	origStop := systemdStopUnit
	t.Cleanup(func() { systemdStopUnit = origStop })
	systemdStopUnit = func(unit string) error {
		return errors.New("permission denied")
	}

	err := DeleteKubernetesEngineEtcdUnit("demo")
	if err == nil {
		t.Fatalf("expected error to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("error = %v, want it to contain %q", err, "permission denied")
	}
}

// systemctlが「ユニット不在」を示すメッセージで失敗した場合は、ユニットファイルが
// 既に存在しなくても DeleteKubernetesEngineEtcdUnit が成功する(冪等)ことを検証する。
func TestDeleteKubernetesEngineEtcdUnitIgnoresUnitMissingErrors(t *testing.T) {
	installFakeSystemd(t)

	origStop := systemdStopUnit
	origDisable := systemdDisableUnit
	t.Cleanup(func() {
		systemdStopUnit = origStop
		systemdDisableUnit = origDisable
	})
	systemdStopUnit = func(unit string) error {
		return &systemdUnitMissingError{err: errors.New("Unit " + unit + " not loaded.")}
	}
	systemdDisableUnit = func(unit string) error {
		return &systemdUnitMissingError{err: errors.New("Failed to disable unit: No such file or directory")}
	}

	if err := DeleteKubernetesEngineEtcdUnit("demo"); err != nil {
		t.Fatalf("DeleteKubernetesEngineEtcdUnit() failed: %v", err)
	}
}

func assertCallsEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls = %v, want %v", got, want)
		}
	}
}
