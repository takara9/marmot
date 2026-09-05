package controller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

func TestEnsureKubernetesEngineCloudControllerManagerKubeconfig(t *testing.T) {
	pkiDir := t.TempDir()
	configDir := t.TempDir()

	kubeconfigPath, err := EnsureKubernetesEngineCloudControllerManagerKubeconfig(pkiDir, configDir, "demo", "172.16.90.100", 6443)
	if err != nil {
		t.Fatalf("EnsureKubernetesEngineCloudControllerManagerKubeconfig() failed: %v", err)
	}

	data, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		t.Fatalf("failed to read kubeconfig: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "https://172.16.90.100:6443") {
		t.Errorf("kubeconfig missing expected server URL: %s", content)
	}
	if !strings.Contains(content, "system:cloud-controller-manager") {
		t.Errorf("kubeconfig missing expected user identity: %s", content)
	}
}

func TestIssueAndRevokeKubernetesEngineCloudProviderApiKey(t *testing.T) {
	database := newGatewayTestDatabase(t)
	if err := database.EnsureBootstrapAdmin(); err != nil {
		t.Fatalf("EnsureBootstrapAdmin() failed: %v", err)
	}

	keyID, rawToken, err := issueKubernetesEngineCloudProviderApiKey(database, "cloud-controller-manager for test-cluster")
	if err != nil {
		t.Fatalf("issueKubernetesEngineCloudProviderApiKey() failed: %v", err)
	}
	if strings.TrimSpace(keyID) == "" {
		t.Error("expected non-empty key ID")
	}
	if strings.TrimSpace(rawToken) == "" {
		t.Error("expected non-empty raw token")
	}

	if err := revokeKubernetesEngineCloudProviderApiKey(database, keyID); err != nil {
		t.Fatalf("revokeKubernetesEngineCloudProviderApiKey() failed: %v", err)
	}

	// 二重失効はErrNotFoundとして吸収され、エラーにならないこと。
	if err := revokeKubernetesEngineCloudProviderApiKey(database, keyID); err != nil {
		t.Fatalf("revokeKubernetesEngineCloudProviderApiKey() on already-revoked key failed: %v", err)
	}
}

func TestProvisionKubernetesEngineCloudControllerManager(t *testing.T) {
	database := newGatewayTestDatabase(t)
	if err := database.EnsureBootstrapAdmin(); err != nil {
		t.Fatalf("EnsureBootstrapAdmin() failed: %v", err)
	}

	origConfig := marmotd.CurrentConfig()
	marmotd.SetRuntimeConfig(&marmotd.MarmotdConfig{
		APIListenAddr:                 "0.0.0.0:8750",
		APIAdvertiseHostBridgeAddress: "192.168.1.10",
	})
	t.Cleanup(func() { marmotd.SetRuntimeConfig(origConfig) })

	recorder := &fakeSystemdRecorder{}
	origReload := systemdDaemonReload
	origEnable := systemdEnableUnit
	origDisable := systemdDisableUnit
	origStart := systemdStartUnit
	origStop := systemdStopUnit
	origUnitDir := controlPlaneSystemdUnitDir
	systemdDaemonReload = func() error { return recorder.record("daemon-reload") }
	systemdEnableUnit = func(unit string) error { return recorder.record("enable:" + unit) }
	systemdDisableUnit = func(unit string) error { return recorder.record("disable:" + unit) }
	systemdStartUnit = func(unit string) error { return recorder.record("start:" + unit) }
	systemdStopUnit = func(unit string) error { return recorder.record("stop:" + unit) }
	controlPlaneSystemdUnitDir = t.TempDir()
	t.Cleanup(func() {
		systemdDaemonReload = origReload
		systemdEnableUnit = origEnable
		systemdDisableUnit = origDisable
		systemdStartUnit = origStart
		systemdStopUnit = origStop
		controlPlaneSystemdUnitDir = origUnitDir
	})

	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		Metadata: api.Metadata{Name: "demo-ccm"},
		Spec:     api.KubernetesEngineSpec{Nodes: 1, Version: "1.30"},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}

	pkiDir := t.TempDir()
	configDir := t.TempDir()

	if err := ProvisionKubernetesEngineCloudControllerManager(database, configDir, pkiDir, &marmotd.MKEConfig{}, ke, "172.16.90.100", 6443); err != nil {
		t.Fatalf("ProvisionKubernetesEngineCloudControllerManager() failed: %v", err)
	}

	unitName := KubernetesEngineCloudControllerManagerUnitName(ke.Metadata.Name)
	started := false
	for _, call := range recorder.calls {
		if call == "start:"+unitName {
			started = true
		}
	}
	if !started {
		t.Errorf("expected systemd start call for %s, got calls=%v", unitName, recorder.calls)
	}

	unitContent, err := os.ReadFile(filepath.Join(controlPlaneSystemdUnitDir, unitName))
	if err != nil {
		t.Fatalf("failed to read generated unit file: %v", err)
	}
	for _, want := range []string{
		"--kubernetes-engine-id=" + api.KubernetesEngineID(ke),
		"--internal-network=" + kubernetesEngineNetworkName(ke),
		"--external-network=",
	} {
		if !strings.Contains(string(unitContent), want) {
			t.Errorf("unit file missing expected arg %q, content=%s", want, unitContent)
		}
	}
	for _, notWant := range []string{"--region=", "--zone="} {
		if strings.Contains(string(unitContent), notWant) {
			t.Errorf("unit file should not contain %q when unset, content=%s", notWant, unitContent)
		}
	}

	updated, err := database.GetKubernetesEngineById(api.KubernetesEngineID(ke))
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Metadata.Labels == nil {
		t.Fatal("expected Metadata.Labels to be set")
	}
	keyID, ok := (*updated.Metadata.Labels)[db.KubernetesEngineCloudProviderLabelApiKeyID].(string)
	if !ok || strings.TrimSpace(keyID) == "" {
		t.Error("expected non-empty cloud provider API key ID label")
	}

	apiKeyPath := filepath.Join(configDir, ke.Metadata.Name, kubernetesEngineCloudProviderApiKeyFileName)
	apiKeyData, err := os.ReadFile(apiKeyPath)
	if err != nil {
		t.Fatalf("failed to read API key file: %v", err)
	}
	if strings.TrimSpace(string(apiKeyData)) == "" {
		t.Error("expected non-empty API key file content")
	}

	if err := revokeKubernetesEngineCloudProviderApiKeyForEngine(database, updated); err != nil {
		t.Fatalf("revokeKubernetesEngineCloudProviderApiKeyForEngine() failed: %v", err)
	}
}

func TestProvisionKubernetesEngineCloudControllerManagerRegionZone(t *testing.T) {
	database := newGatewayTestDatabase(t)
	if err := database.EnsureBootstrapAdmin(); err != nil {
		t.Fatalf("EnsureBootstrapAdmin() failed: %v", err)
	}

	origConfig := marmotd.CurrentConfig()
	marmotd.SetRuntimeConfig(&marmotd.MarmotdConfig{
		APIListenAddr:                 "0.0.0.0:8750",
		APIAdvertiseHostBridgeAddress: "192.168.1.10",
	})
	t.Cleanup(func() { marmotd.SetRuntimeConfig(origConfig) })

	recorder := &fakeSystemdRecorder{}
	origReload := systemdDaemonReload
	origEnable := systemdEnableUnit
	origDisable := systemdDisableUnit
	origStart := systemdStartUnit
	origStop := systemdStopUnit
	origUnitDir := controlPlaneSystemdUnitDir
	systemdDaemonReload = func() error { return recorder.record("daemon-reload") }
	systemdEnableUnit = func(unit string) error { return recorder.record("enable:" + unit) }
	systemdDisableUnit = func(unit string) error { return recorder.record("disable:" + unit) }
	systemdStartUnit = func(unit string) error { return recorder.record("start:" + unit) }
	systemdStopUnit = func(unit string) error { return recorder.record("stop:" + unit) }
	controlPlaneSystemdUnitDir = t.TempDir()
	t.Cleanup(func() {
		systemdDaemonReload = origReload
		systemdEnableUnit = origEnable
		systemdDisableUnit = origDisable
		systemdStartUnit = origStart
		systemdStopUnit = origStop
		controlPlaneSystemdUnitDir = origUnitDir
	})

	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		Metadata: api.Metadata{Name: "demo-ccm-zone"},
		Spec:     api.KubernetesEngineSpec{Nodes: 1, Version: "1.30"},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}

	pkiDir := t.TempDir()
	configDir := t.TempDir()

	mkeConf := &marmotd.MKEConfig{CloudProviderRegion: "region-a", CloudProviderZone: "zone-a"}
	if err := ProvisionKubernetesEngineCloudControllerManager(database, configDir, pkiDir, mkeConf, ke, "172.16.90.100", 6443); err != nil {
		t.Fatalf("ProvisionKubernetesEngineCloudControllerManager() failed: %v", err)
	}

	unitName := KubernetesEngineCloudControllerManagerUnitName(ke.Metadata.Name)
	unitContent, err := os.ReadFile(filepath.Join(controlPlaneSystemdUnitDir, unitName))
	if err != nil {
		t.Fatalf("failed to read generated unit file: %v", err)
	}
	for _, want := range []string{"--region=region-a", "--zone=zone-a"} {
		if !strings.Contains(string(unitContent), want) {
			t.Errorf("unit file missing expected arg %q, content=%s", want, unitContent)
		}
	}
}

func TestUpdateKubernetesEngineCloudProviderApiKeyID(t *testing.T) {
	database := newGatewayTestDatabase(t)

	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		Metadata: api.Metadata{Name: "demo-ccm-label"},
		Spec:     api.KubernetesEngineSpec{Nodes: 1, Version: "1.30"},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)

	if err := database.UpdateKubernetesEngineCloudProviderApiKeyID(id, "key-1"); err != nil {
		t.Fatalf("UpdateKubernetesEngineCloudProviderApiKeyID() failed: %v", err)
	}
	got, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if got.Metadata.Labels == nil {
		t.Fatal("expected Metadata.Labels to be set")
	}
	if v, _ := (*got.Metadata.Labels)[db.KubernetesEngineCloudProviderLabelApiKeyID].(string); v != "key-1" {
		t.Errorf("label = %q, want %q", v, "key-1")
	}

	// 既存ラベルを保持したまま更新できること(他ラベルの上書き消失がないこと)。
	if err := database.UpdateKubernetesEngineCloudProviderApiKeyID(id, "key-2"); err != nil {
		t.Fatalf("UpdateKubernetesEngineCloudProviderApiKeyID() second call failed: %v", err)
	}
	got, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if v, _ := (*got.Metadata.Labels)[db.KubernetesEngineCloudProviderLabelApiKeyID].(string); v != "key-2" {
		t.Errorf("label after update = %q, want %q", v, "key-2")
	}
}

func TestRevokeKubernetesEngineCloudProviderApiKeyForEngineNoLabels(t *testing.T) {
	database := newGatewayTestDatabase(t)

	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		Metadata: api.Metadata{Name: "demo-ccm-nolabel"},
		Spec:     api.KubernetesEngineSpec{Nodes: 1, Version: "1.30"},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}

	// Metadata.Labelsが未設定のクラスタに対しては何もせずエラーにならないこと。
	if err := revokeKubernetesEngineCloudProviderApiKeyForEngine(database, ke); err != nil {
		t.Fatalf("revokeKubernetesEngineCloudProviderApiKeyForEngine() with nil labels failed: %v", err)
	}
}
