package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

func TestKubernetesEngineVMEndToEnd(t *testing.T) {
	if os.Getenv("MARMOT_RUN_MKE_VM_E2E") != "1" {
		t.Skip("set MARMOT_RUN_MKE_VM_E2E=1 to run the KubernetesEngine VM E2E test")
	}

	for _, command := range []string{"ansible-playbook", "curl", "docker", "ip", "systemctl", "virsh"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Fatalf("required command %q is unavailable: %v", command, err)
		}
	}
	if output, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Fatalf("docker daemon is unavailable: %v (output=%s)", err, strings.TrimSpace(string(output)))
	}

	nodeName := fmt.Sprintf("mke-e2e-%d", time.Now().Unix())
	clusterName := fmt.Sprintf("ci-%d", time.Now().Unix())
	endpoint := strings.TrimSpace(os.Getenv("MARMOT_TEST_ETCD_ENDPOINT"))
	if endpoint == "" {
		endpoint = startGatewayTestEtcdContainer(t)
	}

	ma, err := marmotd.NewMarmot(nodeName, endpoint)
	if err != nil {
		t.Fatalf("NewMarmot() failed: %v", err)
	}
	t.Cleanup(func() { _ = ma.Db.Close() })
	if err := ma.CollectAndUpdateHostStatus(); err != nil {
		t.Fatalf("CollectAndUpdateHostStatus() failed: %v", err)
	}

	imageID, err := prepareKubernetesEngineE2EImage(ma, nodeName)
	if err != nil {
		if imageID != "" {
			_ = ma.DeleteImageManage(imageID)
		}
		t.Fatalf("failed to prepare Ubuntu 24.04 image: %v", err)
	}

	var (
		engine            api.KubernetesEngine
		networkController *controller
		volumeController  *controller
		vmController      *controller
		mkeController     *kubernetesEngineController
		stopHostStatus    context.CancelFunc
		hostStatusDone    chan struct{}
	)
	t.Cleanup(func() {
		if mkeController != nil {
			mkeController.Stop()
		}
		if stopHostStatus != nil {
			stopHostStatus()
			<-hostStatusDone
		}
		cleanupKubernetesEngineVMEndToEnd(t, ma, vmController, volumeController, networkController, engine, imageID)
	})

	networkController, err = StartNetController(nodeName, endpoint, 1)
	if err != nil {
		t.Fatalf("StartNetController() failed: %v", err)
	}
	volumeController, err = StartVolController(nodeName, endpoint, 1)
	if err != nil {
		t.Fatalf("StartVolController() failed: %v", err)
	}
	vmController, err = StartVmController(nodeName, endpoint, 1)
	if err != nil {
		t.Fatalf("StartVmController() failed: %v", err)
	}
	mkeController, err = StartKubernetesEngineController(nodeName, endpoint, "")
	if err != nil {
		t.Fatalf("StartKubernetesEngineController() failed: %v", err)
	}

	hostStatusContext, cancelHostStatus := context.WithCancel(context.Background())
	stopHostStatus = cancelHostStatus
	hostStatusDone = make(chan struct{})
	go refreshKubernetesEngineE2EHostStatus(hostStatusContext, hostStatusDone, ma)

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
		},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}

	deadline := time.Now().Add(40 * time.Minute)
	expectedNodeName := kubernetesEngineNodeName(engine, 0)
	for time.Now().Before(deadline) {
		current, getErr := ma.Db.GetKubernetesEngineById(api.KubernetesEngineID(engine))
		if getErr != nil {
			t.Fatalf("GetKubernetesEngineById() failed: %v", getErr)
		}
		engine = current
		if bridgeName, err := failOnKubernetesEngineE2ENetworkError(ma.Db, engine); err != nil {
			logKubernetesEngineE2EOVSDiagnostics(t, bridgeName)
			t.Fatal(err)
		}
		if err := failOnKubernetesEngineE2EServerError(ma.Db, engine); err != nil {
			t.Fatal(err)
		}
		registered, queryErr := kubernetesEngineE2ENodeRegistered(current, expectedNodeName)
		if queryErr == nil && registered {
			t.Logf("Kubernetes node %s registered successfully", expectedNodeName)
			return
		}
		time.Sleep(10 * time.Second)
	}

	message := ""
	if engine.Status != nil && engine.Status.Message != nil {
		message = *engine.Status.Message
	}
	t.Fatalf("timed out waiting for Kubernetes node %s to register; engine status=%+v message=%q", expectedNodeName, engine.Status, message)
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

func failOnKubernetesEngineE2ENetworkError(database *db.Database, engine api.KubernetesEngine) (string, error) {
	networks, err := database.GetVirtualNetworks()
	if err != nil {
		return "", err
	}
	expectedName := kubernetesEngineNetworkName(engine)
	for _, network := range networks {
		if network.Metadata.Name != expectedName || network.Status == nil || network.Status.StatusCode != db.NETWORK_ERROR {
			continue
		}
		bridgeName := ""
		if network.Spec.BridgeName != nil {
			bridgeName = strings.TrimSpace(*network.Spec.BridgeName)
		}
		message := ""
		if network.Status.Message != nil {
			message = strings.TrimSpace(*network.Status.Message)
		}
		return bridgeName, fmt.Errorf("KubernetesEngine network %s entered ERROR: %s", expectedName, message)
	}
	return "", nil
}

func logKubernetesEngineE2EOVSDiagnostics(t *testing.T, bridgeName string) {
	t.Helper()
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
		t.Logf("OVS diagnostic: %s\nerror=%v\n%s", strings.Join(command, " "), err, strings.TrimSpace(string(output)))
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

func cleanupKubernetesEngineVMEndToEnd(t *testing.T, ma *marmotd.Marmot, vmController, volumeController, networkController *controller, engine api.KubernetesEngine, imageID string) {
	t.Helper()
	if api.KubernetesEngineID(engine) != "" {
		if current, err := ma.Db.GetKubernetesEngineById(api.KubernetesEngineID(engine)); err == nil {
			engine = current
		}
		servers, err := findKubernetesEngineNodeServers(ma.Db, engine)
		if err != nil {
			t.Errorf("cleanup: failed to list node servers: %v", err)
		}
		for _, server := range servers {
			if err := ma.Db.SetDeleteTimestamp(api.ServerID(server)); err != nil && !errors.Is(err, db.ErrNotFound) {
				t.Errorf("cleanup: failed to mark server %s for deletion: %v", server.Metadata.Name, err)
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
			t.Errorf("cleanup: %d KubernetesEngine node server(s) remain", len(servers))
		}
		if engine.Status != nil && engine.Status.ControlPlaneIpAddress != nil {
			if err := DeprovisionKubernetesEngineControlPlane(ma.Db, engine); err != nil {
				t.Errorf("cleanup: failed to deprovision control plane: %v", err)
			}
		}
		if network, err := ma.Db.GetVirtualNetworkByName(kubernetesEngineNetworkName(engine)); err == nil {
			if err := ma.DeleteVirtualNetwork(api.VirtualNetworkID(network)); err != nil {
				t.Errorf("cleanup: failed to delete KubernetesEngine network: %v", err)
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
			t.Errorf("cleanup: failed to delete image %s: %v", imageID, err)
		}
	}
}
