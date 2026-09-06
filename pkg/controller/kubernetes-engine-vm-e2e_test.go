package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

// フェーズごとにItを分割し、Ordered経由で失敗箇所以降のItを自動スキップさせることで問題箇所を特定しやすくする。
var _ = Describe("KubernetesEngine VM E2E", Ordered, func() {
	var (
		nodeName          string
		clusterName       string
		endpoint          string
		expectedNodeName  string
		ma                *marmotd.Marmot
		imageID           string
		engine            api.KubernetesEngine
		networkController *controller
		volumeController  *controller
		vmController      *controller
		mkeController     *kubernetesEngineController
		mkeConfigPath     string
		stopHostStatus    context.CancelFunc
		hostStatusDone    chan struct{}
	)

	BeforeAll(func() {
		if os.Getenv("MARMOT_RUN_MKE_VM_E2E") != "1" {
			Skip("set MARMOT_RUN_MKE_VM_E2E=1 to run the KubernetesEngine VM E2E test")
		}

		for _, command := range []string{"ansible-playbook", "curl", "docker", "ip", "systemctl", "virsh"} {
			_, err := exec.LookPath(command)
			Expect(err).NotTo(HaveOccurred(), "required command %q is unavailable", command)
		}
		output, err := exec.Command("docker", "info").CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "docker daemon is unavailable (output=%s)", strings.TrimSpace(string(output)))

		nodeName = fmt.Sprintf("mke-e2e-%d", time.Now().Unix())
		clusterName = fmt.Sprintf("ci-%d", time.Now().Unix())
		endpoint = strings.TrimSpace(os.Getenv("MARMOT_TEST_ETCD_ENDPOINT"))
		if endpoint == "" {
			endpoint = startKubernetesEngineE2EEtcdContainer()
		}

		ma, err = marmotd.NewMarmot(nodeName, endpoint)
		Expect(err).NotTo(HaveOccurred())
		Expect(ma.CollectAndUpdateHostStatus()).To(Succeed())

		mkeConfigPath = writeKubernetesEngineE2EMKEConfig()
	})

	AfterAll(func() {
		if mkeController != nil {
			mkeController.Stop()
		}
		if stopHostStatus != nil {
			stopHostStatus()
			<-hostStatusDone
		}
		cleanupKubernetesEngineVMEndToEnd(ma, mkeController, vmController, volumeController, networkController, engine, imageID)
		if ma != nil {
			_ = ma.Db.Close()
		}
	})

	It("prepares the Ubuntu 24.04 base image", func() {
		By("preparing the Ubuntu 24.04 base image for node " + nodeName)
		stopProgress := startKubernetesEngineE2EElapsedProgress("preparing Ubuntu 24.04 base image")
		defer stopProgress()
		var err error
		imageID, err = prepareKubernetesEngineE2EImage(ma, nodeName)
		Expect(err).NotTo(HaveOccurred())
	})

	It("starts the network, volume, VM, and KubernetesEngine controllers", func() {
		By("starting the network, volume, VM, and KubernetesEngine controllers")
		var err error
		networkController, err = StartNetController(nodeName, endpoint, 1)
		Expect(err).NotTo(HaveOccurred())
		volumeController, err = StartVolController(nodeName, endpoint, 1)
		Expect(err).NotTo(HaveOccurred())
		vmController, err = StartVmController(nodeName, endpoint, 1)
		Expect(err).NotTo(HaveOccurred())
		mkeController, err = StartKubernetesEngineController(nodeName, endpoint, mkeConfigPath)
		Expect(err).NotTo(HaveOccurred())

		hostStatusContext, cancelHostStatus := context.WithCancel(context.Background())
		stopHostStatus = cancelHostStatus
		hostStatusDone = make(chan struct{})
		go refreshKubernetesEngineE2EHostStatus(hostStatusContext, hostStatusDone, ma)
	})

	It("creates the KubernetesEngine cluster resource", func() {
		By("creating the KubernetesEngine cluster resource " + clusterName)
		var err error
		engine, err = ma.Db.CreateKubernetesEngine(api.KubernetesEngine{
			ApiVersion: "v1",
			Kind:       "KubernetesEngine",
			Metadata: api.Metadata{
				Name:     clusterName,
				NodeName: util.StringPtr(nodeName),
			},
			Spec: api.KubernetesEngineSpec{
				Version: "1.36",
				Nodes:   1,
				// host-bridge IPAM未設定のCIランナーでも動くよう、external=defaultを明示する。
				NodeSpec: &api.KubernetesEngineNodeSpec{
					Network: &api.KubernetesEngineNodeNetwork{External: util.StringPtr("default")},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		expectedNodeName = kubernetesEngineNodeName(engine, 0)
	})

	It("provisions the KubernetesEngine dedicated network", func() {
		expectedName := kubernetesEngineNetworkName(engine)
		By("waiting for KubernetesEngine network " + expectedName + " to become active")
		nextProgressLog := time.Now()
		Eventually(func(g Gomega) int {
			networks, err := ma.Db.GetVirtualNetworks()
			g.Expect(err).NotTo(HaveOccurred())
			for _, network := range networks {
				if network.Metadata.Name != expectedName || network.Status == nil {
					continue
				}
				if network.Status.StatusCode == db.NETWORK_ERROR {
					bridgeName := ""
					if network.Spec.BridgeName != nil {
						bridgeName = strings.TrimSpace(*network.Spec.BridgeName)
					}
					logKubernetesEngineE2EOVSDiagnostics(bridgeName)
					message := ""
					if network.Status.Message != nil {
						message = strings.TrimSpace(*network.Status.Message)
					}
					StopTrying(fmt.Sprintf("KubernetesEngine network %s entered ERROR: %s", expectedName, message)).Now()
				}
				logKubernetesEngineE2EProgress(&nextProgressLog, "waiting for network %s; status=%d", expectedName, network.Status.StatusCode)
				return network.Status.StatusCode
			}
			logKubernetesEngineE2EProgress(&nextProgressLog, "network %s not created yet", expectedName)
			return db.NETWORK_PENDING
		}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Equal(db.NETWORK_ACTIVE))
	})

	It("provisions the KubernetesEngine control plane", func() {
		By("waiting for the KubernetesEngine control plane to become healthy")
		nextProgressLog := time.Now()
		Eventually(func(g Gomega) error {
			current, err := ma.Db.GetKubernetesEngineById(api.KubernetesEngineID(engine))
			g.Expect(err).NotTo(HaveOccurred())
			engine = current
			if err := failOnKubernetesEngineE2EServerError(ma.Db, engine); err != nil {
				StopTrying(err.Error()).Now()
			}
			if engine.Status == nil || engine.Status.ControlPlaneIpAddress == nil || engine.Status.ApiServerPort == nil {
				logKubernetesEngineE2EProgress(&nextProgressLog, "control plane address not yet allocated; engine message=%q", kubernetesEngineE2EStatusMessage(engine))
				return fmt.Errorf("control plane address not yet allocated")
			}
			trimmedClusterName := strings.TrimSpace(engine.Metadata.Name)
			caPath, _ := KubernetesEngineCAPaths(DefaultKubernetesEnginePkiDir, trimmedClusterName)
			namespace, _, _, err := KubernetesEngineControlPlaneNetworkNames(trimmedClusterName)
			if err != nil {
				return err
			}
			healthErr := CheckKubernetesEngineControlPlaneHealth(namespace, caPath, *engine.Status.ControlPlaneIpAddress, *engine.Status.ApiServerPort)
			logKubernetesEngineE2EProgress(&nextProgressLog, "waiting for control plane %s:%d health check; last error=%v", *engine.Status.ControlPlaneIpAddress, *engine.Status.ApiServerPort, healthErr)
			return healthErr
		}).WithTimeout(10 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())
	})

	It("provisions the node VM and registers it with the Kubernetes API", func() {
		By("waiting for Kubernetes node " + expectedNodeName + " to register")
		nextProgressLog := time.Now()
		Eventually(func(g Gomega) error {
			current, err := ma.Db.GetKubernetesEngineById(api.KubernetesEngineID(engine))
			g.Expect(err).NotTo(HaveOccurred())
			engine = current
			if err := failOnKubernetesEngineE2EServerError(ma.Db, engine); err != nil {
				StopTrying(err.Error()).Now()
			}
			registered, queryErr := kubernetesEngineE2ENodeRegistered(engine, expectedNodeName)
			logKubernetesEngineE2EProgress(&nextProgressLog, "waiting for Kubernetes node %s; registered=%v query error=%v engine message=%q", expectedNodeName, registered, queryErr, kubernetesEngineE2EStatusMessage(engine))
			if queryErr != nil {
				return queryErr
			}
			if !registered {
				return fmt.Errorf("Kubernetes node %s not yet registered; engine message=%q", expectedNodeName, kubernetesEngineE2EStatusMessage(engine))
			}
			return nil
		}).WithTimeout(25 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	})
})

// writeKubernetesEngineE2EMKEConfig は既存の /etc/marmot/mke.json (存在すればその内容) をベースに、
// control_plane_bind_address を br0 の実IPアドレスで上書きした一時設定ファイルを作成し、そのパスを返す。
// CIランナー側で control_plane_bind_address を事前設定しなくても、フェーズ10のホストアクセス経路を
// 検証できるようにするための、E2Eテスト専用の自動検出処理。
func writeKubernetesEngineE2EMKEConfig() string {
	const controlPlaneBindInterface = "br0"

	iface, err := net.InterfaceByName(controlPlaneBindInterface)
	Expect(err).NotTo(HaveOccurred(), "failed to look up interface %q for control_plane_bind_address", controlPlaneBindInterface)

	addrs, err := iface.Addrs()
	Expect(err).NotTo(HaveOccurred())

	var hostBindAddress string
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil {
			continue
		}
		hostBindAddress = ipNet.IP.String()
		break
	}
	Expect(hostBindAddress).NotTo(BeEmpty(), "interface %q has no usable IPv4 address", controlPlaneBindInterface)

	mkeConf, err := marmotd.LoadMKEConfig(marmotd.DefaultMKEConfigPath)
	Expect(err).NotTo(HaveOccurred())
	mkeConf.ControlPlaneBindAddress = hostBindAddress

	configBytes, err := json.Marshal(mkeConf)
	Expect(err).NotTo(HaveOccurred())

	configPath := filepath.Join(GinkgoT().TempDir(), "mke.json")
	Expect(os.WriteFile(configPath, configBytes, 0o644)).To(Succeed())
	return configPath
}

func startKubernetesEngineE2EEtcdContainer() string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred(), "net.Listen() failed while reserving test port")
	port := listener.Addr().(*net.TCPAddr).Port
	Expect(listener.Close()).To(Succeed())

	output, err := exec.Command("docker", "run", "-d", "--rm", "-p", fmt.Sprintf("%d:2379", port), "ghcr.io/takara9/etcd:3.6.5").CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "failed to start etcd test container (output=%s)", strings.TrimSpace(string(output)))
	containerID := strings.TrimSpace(string(output))
	DeferCleanup(func() {
		_, _ = exec.Command("docker", "stop", containerID).CombinedOutput()
	})

	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
	Eventually(func() error {
		database, err := db.NewDatabase(endpoint)
		if err != nil {
			return err
		}
		return database.Close()
	}).WithTimeout(20 * time.Second).WithPolling(300 * time.Millisecond).Should(Succeed())
	return endpoint
}

func prepareKubernetesEngineE2EImage(ma *marmotd.Marmot, nodeName string) (string, error) {
	const imageName = "ubuntu24.04"
	imageURL := strings.TrimSpace(os.Getenv("MARMOT_MKE_E2E_IMAGE_URL"))
	if imageURL == "" {
		imageURL = "http://hmc/ubuntu-24.04-server-cloudimg-amd64.img"
	}
	imageID, err := ma.Db.MakeImageEntryFromURLWithNode(imageName, imageURL, nodeName)
	if err != nil {
		return "", err
	}
	image, err := ma.Db.GetImage(imageID)
	if err != nil {
		return imageID, err
	}
	image.Spec.OsName = util.StringPtr("ubuntu")
	image.Spec.OsVersion = util.StringPtr("24.04")
	if err := ma.Db.UpdateImage(imageID, image); err != nil {
		return imageID, err
	}
	_, err = ma.CreateNewImageManage(imageID)
	return imageID, err
}

func refreshKubernetesEngineE2EHostStatus(ctx context.Context, done chan<- struct{}, ma *marmotd.Marmot) {
	defer close(done)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = ma.CollectAndUpdateHostStatus()
		}
	}
}

// logKubernetesEngineE2EProgress は-ginkgo.vでリアルタイム表示されるGinkgoWriterへ、長時間のEventuallyポーリング中に
// 経過を1分間隔で出力し、CIログが長時間無出力になるのを防ぐ。
func logKubernetesEngineE2EProgress(nextLogAt *time.Time, format string, args ...any) {
	if time.Now().Before(*nextLogAt) {
		return
	}
	GinkgoWriter.Printf(format+"\n", args...)
	*nextLogAt = time.Now().Add(time.Minute)
}

// startKubernetesEngineE2EElapsedProgress は、Eventuallyによるポーリングを伴わない単発の長時間処理
// （画像ダウンロード・カスタマイズ等）でもCIログが無出力にならないよう、1分間隔で経過時間をGinkgoWriter
// へ出力するティッカーを開始する。呼び出し側は処理完了後に返り値の停止関数を呼び出すこと。
func startKubernetesEngineE2EElapsedProgress(label string) func() {
	start := time.Now()
	done := make(chan struct{})
	ticker := time.NewTicker(time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				GinkgoWriter.Printf("%s: still in progress (elapsed=%s)\n", label, time.Since(start).Round(time.Second))
			}
		}
	}()
	return func() { close(done) }
}

func kubernetesEngineE2EStatusMessage(engine api.KubernetesEngine) string {
	if engine.Status == nil || engine.Status.Message == nil {
		return ""
	}
	return *engine.Status.Message
}

func logKubernetesEngineE2EOVSDiagnostics(bridgeName string) {
	commands := [][]string{
		{"ovs-vsctl", "show"},
		{"ovs-vsctl", "list", "Bridge"},
		{"ovs-vsctl", "list", "Interface"},
		{"ovs-appctl", "-t", "ovs-vswitchd", "dpif/show"},
		{"ip", "-details", "link", "show"},
		{"journalctl", "--no-pager", "-n", "200", "-u", "openvswitch-switch", "-u", "ovs-vswitchd"},
	}
	if bridgeName != "" {
		commands = append(commands, []string{"ovs-vsctl", "get", "Interface", bridgeName, "error"})
	}
	for _, command := range commands {
		output, err := exec.Command(command[0], command[1:]...).CombinedOutput()
		GinkgoWriter.Printf("OVS diagnostic: %s\nerror=%v\n%s\n", strings.Join(command, " "), err, strings.TrimSpace(string(output)))
	}
}

func failOnKubernetesEngineE2EServerError(database *db.Database, engine api.KubernetesEngine) error {
	servers, err := findKubernetesEngineNodeServers(database, engine)
	if err != nil {
		return err
	}
	for _, server := range servers {
		if server.Status != nil && server.Status.StatusCode == db.SERVER_ERROR {
			message := ""
			if server.Status.Message != nil {
				message = *server.Status.Message
			}
			return fmt.Errorf("KubernetesEngine node %s entered ERROR: %s", server.Metadata.Name, message)
		}
	}
	return nil
}

func kubernetesEngineE2ENodeRegistered(engine api.KubernetesEngine, expectedName string) (bool, error) {
	if engine.Status == nil || engine.Status.ControlPlaneIpAddress == nil || engine.Status.ApiServerPort == nil {
		return false, nil
	}
	clusterName := strings.TrimSpace(engine.Metadata.Name)
	adminCertPath, adminKeyPath, err := IssueKubernetesEngineCertificate(DefaultKubernetesEnginePkiDir, clusterName, KubernetesEngineCertRequest{
		Name:          "e2e-admin",
		CommonName:    "mke-e2e-admin",
		Organizations: []string{"system:masters"},
		Usage:         KubernetesEngineCertUsageClient,
	})
	if err != nil {
		return false, err
	}
	caPath, _ := KubernetesEngineCAPaths(DefaultKubernetesEnginePkiDir, clusterName)
	namespace, _, _, err := KubernetesEngineControlPlaneNetworkNames(clusterName)
	if err != nil {
		return false, err
	}
	endpoint := fmt.Sprintf("https://%s:%d/api/v1/nodes", *engine.Status.ControlPlaneIpAddress, *engine.Status.ApiServerPort)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ip", "netns", "exec", namespace, "curl", "--fail", "--silent", "--show-error", "--cacert", caPath, "--cert", adminCertPath, "--key", adminKeyPath, endpoint).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to list Kubernetes nodes: %w (output=%s)", err, strings.TrimSpace(string(output)))
	}
	var nodeList kubernetesNodeList
	if err := json.Unmarshal(output, &nodeList); err != nil {
		return false, err
	}
	for _, item := range nodeList.Items {
		if item.Metadata.Name == expectedName {
			return true, nil
		}
	}
	return false, nil
}

func cleanupKubernetesEngineVMEndToEnd(ma *marmotd.Marmot, mkeController *kubernetesEngineController, vmController, volumeController, networkController *controller, engine api.KubernetesEngine, imageID string) {
	var cleanupErrors []error
	if api.KubernetesEngineID(engine) != "" {
		if current, err := ma.Db.GetKubernetesEngineById(api.KubernetesEngineID(engine)); err == nil {
			engine = current
		}
		servers, err := findKubernetesEngineNodeServers(ma.Db, engine)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to list node servers: %w", err))
		}
		for _, server := range servers {
			if err := ma.Db.SetDeleteTimestamp(api.ServerID(server)); err != nil && !errors.Is(err, db.ErrNotFound) {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to mark server %s for deletion: %w", server.Metadata.Name, err))
			}
		}
		deadline := time.Now().Add(3 * time.Minute)
		for len(servers) > 0 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Second)
			servers, err = findKubernetesEngineNodeServers(ma.Db, engine)
			if err != nil {
				break
			}
		}
		if len(servers) > 0 {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("%d KubernetesEngine node server(s) remain", len(servers)))
		}
		if engine.Status != nil && engine.Status.ControlPlaneIpAddress != nil {
			if err := DeprovisionKubernetesEngineControlPlane(ma.Db, mkeController.mkeConf, engine); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to deprovision control plane: %w", err))
			}
		}
		if network, err := ma.Db.GetVirtualNetworkByName(kubernetesEngineNetworkName(engine)); err == nil {
			if err := ma.DeleteVirtualNetwork(api.VirtualNetworkID(network)); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to delete KubernetesEngine network: %w", err))
			}
		}
	}
	if vmController != nil {
		vmController.Stop()
	}
	if volumeController != nil {
		volumeController.Stop()
	}
	if networkController != nil {
		networkController.Stop()
	}
	if imageID != "" {
		if err := ma.DeleteImageManage(imageID); err != nil && !errors.Is(err, db.ErrNotFound) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to delete image %s: %w", imageID, err))
		}
	}
	if len(cleanupErrors) > 0 {
		Fail(fmt.Sprintf("cleanup failed: %v", errors.Join(cleanupErrors...)))
	}
}
