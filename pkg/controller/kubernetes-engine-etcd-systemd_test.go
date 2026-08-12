package controller

import (
	"errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeSystemdRecorder は systemctl 呼び出しの記録用ヘルパー。
type fakeSystemdRecorder struct {
	calls []string
}

func (f *fakeSystemdRecorder) record(action string) error {
	f.calls = append(f.calls, action)
	return nil
}

func installFakeSystemd() *fakeSystemdRecorder {
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
	etcdSystemdUnitDir = GinkgoT().TempDir()

	DeferCleanup(func() {
		systemdDaemonReload = origReload
		systemdEnableUnit = origEnable
		systemdDisableUnit = origDisable
		systemdStartUnit = origStart
		systemdStopUnit = origStop
		etcdSystemdUnitDir = origUnitDir
	})
	return rec
}

var _ = Describe("KubernetesEngineEtcdUnit", func() {
	It("derives the systemd unit name from the cluster name", func() {
		Expect(KubernetesEngineEtcdUnitName("demo")).To(Equal("mke-etcd-demo.service"))
	})

	It("renders the network namespace into the unit file", func() {
		content := renderKubernetesEngineEtcdUnit(KubernetesEngineEtcdUnitConfig{
			ClusterName:      "demo",
			EtcdBinaryPath:   "/bin/etcd",
			DataDir:          "/var/lib/etcd",
			NetworkNamespace: "mke-demo",
			ClientPort:       23790,
			PeerPort:         23791,
		})
		Expect(content).To(ContainSubstring("NetworkNamespacePath=/run/netns/mke-demo"))
	})

	// クラスタ専用etcdユニットの「作成→起動→停止→削除」の一連のライフサイクルを検証する。
	It("creates, starts, stops and deletes the etcd unit for a cluster (idempotently)", func() {
		rec := installFakeSystemd()
		dataDir := filepath.Join(GinkgoT().TempDir(), "data")

		cfg := KubernetesEngineEtcdUnitConfig{
			ClusterName:    "demo",
			EtcdBinaryPath: "/var/lib/marmot/mke/etcd/v3.6.8-linux-amd64/etcd",
			DataDir:        dataDir,
			ClientPort:     23790,
			PeerPort:       23791,
		}

		// 作成→起動
		Expect(CreateKubernetesEngineEtcdUnit(cfg)).To(Succeed())
		_, err := os.Stat(dataDir)
		Expect(err).NotTo(HaveOccurred())
		unitPath := kubernetesEngineEtcdUnitPath("demo")
		content, err := os.ReadFile(unitPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring(cfg.EtcdBinaryPath))
		Expect(string(content)).To(ContainSubstring("23790"))
		Expect(string(content)).To(ContainSubstring("23791"))

		Expect(rec.calls).To(Equal([]string{"daemon-reload", "enable:mke-etcd-demo.service", "start:mke-etcd-demo.service"}))

		// 停止
		rec.calls = nil
		Expect(StopKubernetesEngineEtcdUnit("demo")).To(Succeed())
		Expect(rec.calls).To(Equal([]string{"stop:mke-etcd-demo.service"}))
		_, err = os.Stat(unitPath)
		Expect(err).NotTo(HaveOccurred())

		// 削除
		rec.calls = nil
		Expect(DeleteKubernetesEngineEtcdUnit("demo")).To(Succeed())
		Expect(rec.calls).To(Equal([]string{"stop:mke-etcd-demo.service", "disable:mke-etcd-demo.service", "daemon-reload"}))
		_, err = os.Stat(unitPath)
		Expect(os.IsNotExist(err)).To(BeTrue())

		// 削除後の再削除は冪等(ユニットファイル不在でもエラーにならない)であること
		rec.calls = nil
		Expect(DeleteKubernetesEngineEtcdUnit("demo")).To(Succeed())
	})

	// DeleteKubernetesEngineEtcdUnit は「ユニット不在」相当の失敗のみ無視し、
	// それ以外の失敗(権限不足等)はユニットファイルが既に存在しない場合でも
	// 呼び出し元に伝播することを検証する。
	It("propagates non-missing-unit errors", func() {
		installFakeSystemd()

		origStop := systemdStopUnit
		DeferCleanup(func() { systemdStopUnit = origStop })
		systemdStopUnit = func(unit string) error {
			return errors.New("permission denied")
		}

		err := DeleteKubernetesEngineEtcdUnit("demo")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("permission denied"))
	})

	// systemctlが「ユニット不在」を示すメッセージで失敗した場合は、ユニットファイルが
	// 既に存在しなくても DeleteKubernetesEngineEtcdUnit が成功する(冪等)ことを検証する。
	It("ignores unit-missing errors", func() {
		installFakeSystemd()

		origStop := systemdStopUnit
		origDisable := systemdDisableUnit
		DeferCleanup(func() {
			systemdStopUnit = origStop
			systemdDisableUnit = origDisable
		})
		systemdStopUnit = func(unit string) error {
			return &systemdUnitMissingError{err: errors.New("Unit " + unit + " not loaded.")}
		}
		systemdDisableUnit = func(unit string) error {
			return &systemdUnitMissingError{err: errors.New("Failed to disable unit: No such file or directory")}
		}

		Expect(DeleteKubernetesEngineEtcdUnit("demo")).To(Succeed())
	})

	// StopKubernetesEngineEtcdUnit はCreate/Deleteと同様に不正なクラスタ名を拒否し、
	// systemctlを呼び出さないことを検証する。
	It("rejects invalid cluster names without calling systemctl", func() {
		rec := installFakeSystemd()

		Expect(StopKubernetesEngineEtcdUnit(" ")).To(HaveOccurred())
		Expect(StopKubernetesEngineEtcdUnit("demo/../etc")).To(HaveOccurred())
		Expect(rec.calls).To(BeEmpty())
	})
})
