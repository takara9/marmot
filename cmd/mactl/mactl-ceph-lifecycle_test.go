package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

var _ = Describe("Ceph volume backed VM lifecycle", Ordered, func() {
	var mockServer *mockServerHandle
	var containerID string
	var testHomeDir string
	var tempDir string
	var apiConfigPath string
	var volumeID string
	var serverID string
	var mactlBin = "./bin/mactl-test"

	BeforeAll(func(specCtx SpecContext) {
		cephMonitor := strings.TrimSpace(os.Getenv("CEPH_IPADDR"))
		if cephMonitor == "" {
			Skip("set CEPH_IPADDR and CEPH_POOL_KEY to enable the Ceph lifecycle smoke test")
		}

		cephKey := strings.TrimSpace(os.Getenv("CEPH_POOL_KEY"))
		if cephKey == "" {
			Skip("set CEPH_IPADDR and CEPH_POOL_KEY to enable the Ceph lifecycle smoke test")
		}

		opts := &slog.HandlerOptions{AddSource: true}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)
		cleanupTestEnvironment()

		var err error
		testHomeDir, err = setupMactlTestHome()
		Expect(err).NotTo(HaveOccurred())
		if err := ensureMactlTestBinary(); err != nil {
			Fail(fmt.Sprintf("Failed to build mactl test binary: %v", err))
		}

		tempDir = GinkgoT().TempDir()
		keyFilePath := filepath.Join(tempDir, "ceph.keyring")
		keyring := cephKey
		if !strings.HasSuffix(keyring, "\n") {
			keyring += "\n"
		}
		Expect(os.WriteFile(keyFilePath, []byte(keyring), 0o600)).To(Succeed())

		baseConfigBytes, err := os.ReadFile("testdata/marmotd.json")
		Expect(err).NotTo(HaveOccurred())
		var configData map[string]json.RawMessage
		Expect(json.Unmarshal(baseConfigBytes, &configData)).To(Succeed())
		configData["ceph_enabled"] = json.RawMessage("true")
		configData["ceph_monitors"] = json.RawMessage(fmt.Sprintf(`["%s"]`, cephMonitor))
		configData["ceph_user"] = json.RawMessage(`"client.marmot"`)
		configData["ceph_key_file"] = json.RawMessage(strconvQuote(keyFilePath))
		configData["ceph_pool_by_class"] = json.RawMessage(`[{"storageClass":"ssd","pool":"marmot-ssd"}]`)

		configBytes, err := json.MarshalIndent(configData, "", "  ")
		Expect(err).NotTo(HaveOccurred())
		apiConfigPath = filepath.Join(tempDir, "marmotd-ceph.json")
		Expect(os.WriteFile(apiConfigPath, configBytes, 0o600)).To(Succeed())

		By("モックサーバー用etcdの起動")
		var etcdEp string
		containerID, etcdEp, err = startEtcdContainer()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %v", err))
		}
		fmt.Printf("Container started with ID: %s\n", containerID)

		By("Ceph 有効のモックサーバー起動")
		mockServer, err = startMockServer(etcdEp, apiConfigPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(loginAsAdmin()).NotTo(HaveOccurred())
	})

	AfterAll(func(specCtx SpecContext) {
		if mockServer != nil {
			mockServer.Stop()
		}
		if strings.TrimSpace(containerID) != "" {
			cmd := exec.Command("docker", "kill", containerID)
			_, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Failed to stop container: %v\n", err)
			}
			cmd = exec.Command("docker", "rm", containerID)
			_, err = cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Failed to remove container: %v\n", err)
			}
		}
		if strings.TrimSpace(testHomeDir) != "" {
			_ = os.RemoveAll(testHomeDir)
		}
		_ = os.Remove("bin/mactl-test")
		_ = os.Remove("/var/actions-runner/_work/marmot/marmot/cmd/mactl/bin/mactl-test")
		cleanupTestEnvironment()
	})

	It("Ceph volume を使う仮想マシンの起動から削除までを通す", func() {
		volumeName := fmt.Sprintf("ceph-lifecycle-%d", time.Now().UnixNano())
		volumeYAML := filepath.Join(tempDir, "ceph-volume.yaml")
		volumeYAMLContent := fmt.Sprintf(`apiVersion: v1
kind: Volume
metadata:
  name: %s
spec:
  kind: data
  type: ceph
  size: 1
  storageClass: ssd
`, volumeName)
		Expect(os.WriteFile(volumeYAML, []byte(volumeYAMLContent), 0o600)).To(Succeed())

		By("Ceph ボリュームの作成")
		createVolumeCmd := exec.Command(mactlBin, "--api", "testdata/.marmot", "volume", "create", "-f", volumeYAML, "--output", "json")
		createVolumeOut, createVolumeErr := createVolumeCmd.CombinedOutput()
		Expect(createVolumeErr).NotTo(HaveOccurred(), "mactl volume create failed: %s", string(createVolumeOut))

		var createdVolume api.Volume
		Expect(json.Unmarshal(createVolumeOut, &createdVolume)).To(Succeed(), "unexpected volume create output: %s", string(createVolumeOut))
		Expect(api.VolumeID(createdVolume)).NotTo(BeEmpty())
		Expect(createdVolume.Status).NotTo(BeNil())
		Expect(createdVolume.Status.Provider).NotTo(BeNil())
		Expect(strings.ToLower(strings.TrimSpace(*createdVolume.Status.Provider))).To(Equal("ceph"))
		Expect(createdVolume.Status.AttachProtocol).NotTo(BeNil())
		Expect(strings.ToLower(strings.TrimSpace(*createdVolume.Status.AttachProtocol))).To(Equal("rbd"))
		Expect(createdVolume.Status.ProviderVolumeId).NotTo(BeNil())
		volumeID = api.VolumeID(createdVolume)

		DeferCleanup(func() {
			if strings.TrimSpace(volumeID) == "" {
				return
			}
			deleteVolumeCmd := exec.Command(mactlBin, "--api", "testdata/.marmot", "volume", "delete", volumeID, "--output", "json")
			deleteVolumeOut, deleteVolumeErr := deleteVolumeCmd.CombinedOutput()
			Expect(deleteVolumeErr).NotTo(HaveOccurred(), "mactl volume delete failed: %s", string(deleteVolumeOut))
		})

		By("Ceph ボリュームを参照する仮想サーバーの作成")
		serverYAML := filepath.Join(tempDir, "ceph-server.yaml")
		serverName := fmt.Sprintf("ceph-server-%d", time.Now().UnixNano())
		serverYAMLContent := fmt.Sprintf(`apiVersion: v1
kind: Server
metadata:
  name: %s
spec:
  cpu: 2
  memory: 2048
  osVariant: ubuntu22.04
  bootVolume:
    spec:
      type: qcow2
  networkInterface:
    - networkname: default
  storage:
    - metadata:
        name: %s
`, serverName, volumeName)

		Expect(os.WriteFile(serverYAML, []byte(serverYAMLContent), 0o600)).To(Succeed())

		createServerCmd := exec.Command(mactlBin, "--api", "testdata/.marmot", "server", "create", "--configfile", serverYAML, "--output", "json")
		createServerOut, createServerErr := createServerCmd.CombinedOutput()
		Expect(createServerErr).NotTo(HaveOccurred(), "mactl server create failed: %s", string(createServerOut))

		var createdServer api.Success
		Expect(json.Unmarshal(createServerOut, &createdServer)).To(Succeed(), "unexpected server create output: %s", string(createServerOut))
		serverID = createdServer.Id
		Expect(strings.TrimSpace(serverID)).NotTo(BeEmpty())

		DeferCleanup(func() {
			if strings.TrimSpace(serverID) == "" {
				return
			}
			deleteServerCmd := exec.Command(mactlBin, "--api", "testdata/.marmot", "server", "delete", serverID, "--output", "json")
			deleteServerOut, deleteServerErr := deleteServerCmd.CombinedOutput()
			Expect(deleteServerErr).NotTo(HaveOccurred(), "mactl server delete failed: %s", string(deleteServerOut))
		})

		By("仮想サーバーが RUNNING になることを確認")
		Eventually(func(g Gomega) {
			cmd := exec.Command(mactlBin, "--api", "testdata/.marmot", "server", "detail", serverID, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			g.Expect(err).NotTo(HaveOccurred(), "mactl server detail failed: %s", string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			g.Expect(err).NotTo(HaveOccurred(), "unexpected server detail output: %s", string(stdoutStderr))
			g.Expect(server.Status).NotTo(BeNil())
			g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_RUNNING)))
			g.Expect(server.Spec.Storage).NotTo(BeNil())
			g.Expect(*server.Spec.Storage).To(HaveLen(1))
			g.Expect((*server.Spec.Storage)[0].Metadata.Name).To(Equal(volumeName))
		}, 120*time.Second, 5*time.Second).Should(Succeed())

		By("仮想サーバーを削除する")
		deleteServerCmd := exec.Command(mactlBin, "--api", "testdata/.marmot", "server", "delete", serverID, "--output", "json")
		deleteServerOut, deleteServerErr := deleteServerCmd.CombinedOutput()
		Expect(deleteServerErr).NotTo(HaveOccurred(), "mactl server delete failed: %s", string(deleteServerOut))

		By("仮想サーバーが削除されたことを確認する")
		Eventually(func(g Gomega) {
			cmd := exec.Command(mactlBin, "--api", "testdata/.marmot", "server", "detail", serverID, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			if err != nil && strings.Contains(strings.ToLower(string(stdoutStderr)), "not found") {
				return
			}
			g.Expect(err).NotTo(HaveOccurred(), "mactl server detail after delete failed: %s", string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			g.Expect(err).NotTo(HaveOccurred(), "unexpected server detail output after delete: %s", string(stdoutStderr))
			g.Expect(server.Status).NotTo(BeNil())
			g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_DELETING)))
		}, 120*time.Second, 5*time.Second).Should(Succeed())

		By("Ceph ボリュームを削除する")
		deleteVolumeCmd := exec.Command(mactlBin, "--api", "testdata/.marmot", "volume", "delete", volumeID, "--output", "json")
		deleteVolumeOut, deleteVolumeErr := deleteVolumeCmd.CombinedOutput()
		Expect(deleteVolumeErr).NotTo(HaveOccurred(), "mactl volume delete failed: %s", string(deleteVolumeOut))

		By("Ceph ボリュームが削除されたことを確認する")
		Eventually(func(g Gomega) {
			cmd := exec.Command(mactlBin, "--api", "testdata/.marmot", "volume", "detail", volumeID, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			if err != nil && strings.Contains(strings.ToLower(string(stdoutStderr)), "not found") {
				return
			}
			g.Expect(err).NotTo(HaveOccurred(), "mactl volume detail after delete failed: %s", string(stdoutStderr))

			var volume api.Volume
			err = json.Unmarshal(stdoutStderr, &volume)
			g.Expect(err).NotTo(HaveOccurred(), "unexpected volume detail output after delete: %s", string(stdoutStderr))
			g.Expect(volume.Status).NotTo(BeNil())
			g.Expect(volume.Status.StatusCode).To(Equal(int(db.VOLUME_DELETING)))
		}, 120*time.Second, 5*time.Second).Should(Succeed())
	})
})

func strconvQuote(value string) json.RawMessage {
	quoted, err := json.Marshal(value)
	Expect(err).NotTo(HaveOccurred())
	return quoted
}