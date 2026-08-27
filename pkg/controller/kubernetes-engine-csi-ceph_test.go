package controller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKubernetesEngineRenderCephCSIConfigMap(t *testing.T) {
	content := `---
apiVersion: v1
kind: ConfigMap
data:
  config.json: |-
    [
      {
        "clusterID": "070aabed-a12a-11f1-8a75-921da53eb49a",
        "rbd": {
           "nodePublishSecretRef": {
             "name": "csi-rbd-secret",
             "namespace": "kube-system"
           }
        },
        "monitors": [
          "192.168.1.170:6789"
        ]
      }
    ]
metadata:
  name: ceph-csi-config
`
	got := kubernetesEngineRenderCephCSIConfigMap(content, "my-cluster", []string{"10.0.0.1:6789", "10.0.0.2:6789"})
	if !strings.Contains(got, `"clusterID": "my-cluster"`) {
		t.Fatalf("clusterID not replaced, got:\n%s", got)
	}
	if !strings.Contains(got, `"monitors": ["10.0.0.1:6789", "10.0.0.2:6789"]`) {
		t.Fatalf("monitors not replaced, got:\n%s", got)
	}
	if !strings.Contains(got, `"name": "csi-rbd-secret"`) {
		t.Fatalf("unrelated content was modified, got:\n%s", got)
	}
}

func TestKubernetesEngineSetYAMLScalar(t *testing.T) {
	content := "apiVersion: storage.k8s.io/v1\nkind: StorageClass\nparameters:\n  clusterID: 070aabed-a12a-11f1-8a75-921da53eb49a\n  pool: rbdpool\n"
	got, err := kubernetesEngineSetYAMLScalar(content, "clusterID", "my-cluster")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "clusterID: my-cluster") {
		t.Fatalf("clusterID not replaced, got:\n%s", got)
	}
	if !strings.Contains(got, "pool: rbdpool") {
		t.Fatalf("unrelated field modified, got:\n%s", got)
	}

	if _, err := kubernetesEngineSetYAMLScalar(content, "doesNotExist", "x"); err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestKubernetesEngineSetYAMLSecretValues(t *testing.T) {
	content := "apiVersion: v1\nkind: Secret\nstringData:\n  userID: rbduser1\n  userKey: AQDGp45qMcwsBhAAGurQXn8l7hf3wU4gOiUNnA==\n"
	got, err := kubernetesEngineSetYAMLSecretValues(content, "admin", "s3cr3tKey==")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "userID: admin") {
		t.Fatalf("userID not replaced, got:\n%s", got)
	}
	if !strings.Contains(got, "userKey: s3cr3tKey==") {
		t.Fatalf("userKey not replaced, got:\n%s", got)
	}
}

func TestPrepareKubernetesEngineCephCSIManifests(t *testing.T) {
	baseDir := t.TempDir()
	mustWriteFile(t, filepath.Join(baseDir, "ceph-conf.yaml"), "kind: ConfigMap\nmetadata:\n  name: ceph-config\n")
	mustWriteFile(t, filepath.Join(baseDir, "csi-config-map.yaml"), `data:
  config.json: |-
    [
      {
        "clusterID": "070aabed-a12a-11f1-8a75-921da53eb49a",
        "monitors": [
          "192.168.1.170:6789"
        ]
      }
    ]
`)
	mustWriteFile(t, filepath.Join(baseDir, "ceph-rbd", "csi-rbd-secret.yaml"), "stringData:\n  userID: rbduser1\n  userKey: AAAA\n")
	mustWriteFile(t, filepath.Join(baseDir, "ceph-rbd", "rbd-storageclass.yaml"), "parameters:\n  clusterID: 070aabed-a12a-11f1-8a75-921da53eb49a\n  pool: rbdpool\n")
	mustWriteFile(t, filepath.Join(baseDir, "ceph-rbd", "csidriver.yaml"), "kind: CSIDriver\n")
	mustWriteFile(t, filepath.Join(baseDir, "ceph-fs", "csi-cephfs-secret.yaml"), "stringData:\n  userID: fsuser1\n  userKey: BBBB\n")
	mustWriteFile(t, filepath.Join(baseDir, "ceph-fs", "cephfs-storageclass.yaml"), "parameters:\n  clusterID: 070aabed-a12a-11f1-8a75-921da53eb49a\n  fsName: cephfs\n")
	mustWriteFile(t, filepath.Join(baseDir, "kms", "vault.yaml"), "kind: Deployment\n")

	clusterDir := filepath.Join(baseDir, "clusters", "test-cluster")
	values := kubernetesEngineCephCSIValues{
		ClusterID:     "my-cluster",
		Monitors:      []string{"10.0.0.1:6789"},
		RBDUserID:     "rbdadmin",
		RBDUserKey:    "rbdkey==",
		CephFSUserID:  "fsadmin",
		CephFSUserKey: "fskey==",
	}
	if err := prepareKubernetesEngineCephCSIManifests(baseDir, clusterDir, values); err != nil {
		t.Fatalf("prepareKubernetesEngineCephCSIManifests: %v", err)
	}

	configMap := mustReadFile(t, filepath.Join(clusterDir, "csi-config-map.yaml"))
	if !strings.Contains(configMap, `"clusterID": "my-cluster"`) || !strings.Contains(configMap, `"monitors": ["10.0.0.1:6789"]`) {
		t.Fatalf("csi-config-map.yaml not rendered correctly:\n%s", configMap)
	}

	rbdSecret := mustReadFile(t, filepath.Join(clusterDir, "ceph-rbd", "csi-rbd-secret.yaml"))
	if !strings.Contains(rbdSecret, "userID: rbdadmin") || !strings.Contains(rbdSecret, "userKey: rbdkey==") {
		t.Fatalf("csi-rbd-secret.yaml not rendered correctly:\n%s", rbdSecret)
	}

	rbdStorageClass := mustReadFile(t, filepath.Join(clusterDir, "ceph-rbd", "rbd-storageclass.yaml"))
	if !strings.Contains(rbdStorageClass, "clusterID: my-cluster") || !strings.Contains(rbdStorageClass, "pool: rbdpool") {
		t.Fatalf("rbd-storageclass.yaml not rendered correctly:\n%s", rbdStorageClass)
	}

	cephfsSecret := mustReadFile(t, filepath.Join(clusterDir, "ceph-fs", "csi-cephfs-secret.yaml"))
	if !strings.Contains(cephfsSecret, "userID: fsadmin") || !strings.Contains(cephfsSecret, "userKey: fskey==") {
		t.Fatalf("csi-cephfs-secret.yaml not rendered correctly:\n%s", cephfsSecret)
	}

	cephfsStorageClass := mustReadFile(t, filepath.Join(clusterDir, "ceph-fs", "cephfs-storageclass.yaml"))
	if !strings.Contains(cephfsStorageClass, "clusterID: my-cluster") {
		t.Fatalf("cephfs-storageclass.yaml not rendered correctly:\n%s", cephfsStorageClass)
	}

	// Files untouched by the doc's substitution list must be copied verbatim.
	if got := mustReadFile(t, filepath.Join(clusterDir, "ceph-rbd", "csidriver.yaml")); got != "kind: CSIDriver\n" {
		t.Fatalf("csidriver.yaml should be copied verbatim, got: %q", got)
	}
	if got := mustReadFile(t, filepath.Join(clusterDir, "kms", "vault.yaml")); got != "kind: Deployment\n" {
		t.Fatalf("vault.yaml should be copied verbatim, got: %q", got)
	}

	// Idempotency: re-running with different values must not overwrite the already-copied dir.
	if err := prepareKubernetesEngineCephCSIManifests(baseDir, clusterDir, kubernetesEngineCephCSIValues{
		ClusterID: "changed", Monitors: []string{"x"}, RBDUserID: "x", RBDUserKey: "x", CephFSUserID: "x", CephFSUserKey: "x",
	}); err != nil {
		t.Fatalf("second call should be a no-op, got error: %v", err)
	}
	if got := mustReadFile(t, filepath.Join(clusterDir, "ceph-rbd", "rbd-storageclass.yaml")); !strings.Contains(got, "clusterID: my-cluster") {
		t.Fatalf("existing cluster dir must not be re-rendered, got:\n%s", got)
	}
}

func TestRemoveKubernetesEngineCephCSIManifests(t *testing.T) {
	baseDir := t.TempDir()
	clusterDir, err := kubernetesEngineCephCSIClusterManifestsDir(baseDir, "test-cluster")
	if err != nil {
		t.Fatalf("kubernetesEngineCephCSIClusterManifestsDir: %v", err)
	}
	if err := os.MkdirAll(clusterDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := removeKubernetesEngineCephCSIManifests(baseDir, "test-cluster"); err != nil {
		t.Fatalf("removeKubernetesEngineCephCSIManifests: %v", err)
	}
	if _, err := os.Stat(clusterDir); !os.IsNotExist(err) {
		t.Fatalf("expected cluster dir to be removed, stat err=%v", err)
	}

	// Removing an already-absent cluster dir must be a no-op, not an error.
	if err := removeKubernetesEngineCephCSIManifests(baseDir, "test-cluster"); err != nil {
		t.Fatalf("removing already-absent dir should be a no-op, got error: %v", err)
	}
}

func TestKubernetesEngineCephCSIClusterManifestsDirRejectsInvalidNames(t *testing.T) {
	if _, err := kubernetesEngineCephCSIClusterManifestsDir("/var/lib/marmot/mke-manifests", "../escape"); err == nil {
		t.Fatal("expected error for path-traversal cluster name, got nil")
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return string(data)
}
