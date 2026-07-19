package marmotd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

var _ = Describe("Ceph integration helpers", Ordered, func() {
	var originalConfig *MarmotdConfig
	var cephTestConfig *MarmotdConfig
	var keyFilePath string

	BeforeAll(func() {
		originalConfig = CurrentConfig()
		var err error
		keyFile, err := os.CreateTemp("", "marmot-ceph-key-*.key")
		Expect(err).NotTo(HaveOccurred())
		keyFilePath = keyFile.Name()
		_, err = keyFile.WriteString("[client.ubuntu]\n\tkey = AQCAD1dq4k4jARAAq7ckh5t6aouhEGokyef0Fg==\n")
		Expect(err).NotTo(HaveOccurred())
		Expect(keyFile.Close()).To(Succeed())

		cephTestConfig = &MarmotdConfig{
			NodeName:     "hv1",
			EtcdURL:      "http://127.0.0.1:2379",
			CephEnabled:  true,
			CephMonitors: []string{"10.1.3.11:6789"},
			CephUser:     "client.ubuntu",
			CephKeyFile:  keyFilePath,
			CephPoolByClass: map[string]string{
				"ssd": "marmot-ssd",
			},
		}
		SetRuntimeConfig(cephTestConfig)
	})

	AfterAll(func() {
		SetRuntimeConfig(originalConfig)
		_ = os.Remove(keyFilePath)
	})

	It("builds a ceph disk spec for VM attach", func() {
		volume := api.Volume{
			Spec: api.VolSpec{
				Type:         util.StringPtr("ceph"),
				Kind:         util.StringPtr("data"),
				Size:         util.IntPtrInt(1),
				StorageClass: util.StringPtr("ssd"),
			},
		}
		api.SetVolumeID(&volume, "abcde")

		disk, err := buildCephDiskSpec(volume, "server-123", "vdb", 11)
		Expect(err).NotTo(HaveOccurred())
		Expect(disk.Type).To(Equal("rbd"))
		Expect(disk.Src).To(Equal("marmot-ssd/vol-abcde"))
		Expect(disk.CephUser).To(Equal("client.ubuntu"))
		Expect(disk.CephMonitors).To(ContainElements("10.1.3.11:6789"))
		Expect(disk.CephSecretUUID).To(Equal(cephSecretUUIDForServer("server-123")))
	})

	It("prefers status providerVolumeId for VM attach disk source", func() {
		volume := api.Volume{
			Spec: api.VolSpec{
				Type:         util.StringPtr("ceph"),
				Kind:         util.StringPtr("data"),
				Size:         util.IntPtrInt(1),
				StorageClass: util.StringPtr("ssd"),
			},
			Status: &api.Status{ProviderVolumeId: util.StringPtr("stored-pool/stored-image")},
		}
		api.SetVolumeID(&volume, "abcde")

		disk, err := buildCephDiskSpec(volume, "server-123", "vdb", 11)
		Expect(err).NotTo(HaveOccurred())
		Expect(disk.Src).To(Equal("stored-pool/stored-image"))
	})

	It("returns an error when monitors become empty after trimming", func() {
		cfg := *CurrentConfig()
		cfg.CephMonitors = []string{"   ", "\t"}
		SetRuntimeConfig(&cfg)
		defer SetRuntimeConfig(cephTestConfig)

		volume := api.Volume{
			Spec: api.VolSpec{
				Type:         util.StringPtr("ceph"),
				Kind:         util.StringPtr("data"),
				Size:         util.IntPtrInt(1),
				StorageClass: util.StringPtr("ssd"),
			},
		}
		api.SetVolumeID(&volume, "abcde")

		_, err := buildCephDiskSpec(volume, "server-123", "vdb", 11)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ceph monitors are required"))
	})

	It("rejects empty server id when building ceph disk spec", func() {
		volume := api.Volume{
			Spec: api.VolSpec{
				Type:         util.StringPtr("ceph"),
				Kind:         util.StringPtr("data"),
				Size:         util.IntPtrInt(1),
				StorageClass: util.StringPtr("ssd"),
			},
		}
		api.SetVolumeID(&volume, "abcde")

		_, err := buildCephDiskSpec(volume, "   ", "vdb", 11)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("server id is required"))
	})

	It("marks ceph storage as node-independent", func() {
		storage := []api.Volume{
			{
				Spec: api.VolSpec{Type: util.StringPtr("ceph")},
			},
			{
				Spec: api.VolSpec{Type: util.StringPtr("qcow2")},
			},
		}
		Expect(hasCephStorage(&storage)).To(BeTrue())
		Expect(hasCephStorage(&[]api.Volume{{Spec: api.VolSpec{Type: util.StringPtr("qcow2")}}})).To(BeFalse())
	})

	It("creates deterministic ceph secret UUIDs", func() {
		first := cephSecretUUIDForServer("server-123")
		second := cephSecretUUIDForServer("server-123")
		different := cephSecretUUIDForServer("server-456")
		Expect(first).To(Equal(second))
		Expect(first).NotTo(Equal(different))
	})

	It("prefers status providerVolumeId when resolving ceph delete targets", func() {
		vol := api.Volume{}
		vol.Status = &api.Status{ProviderVolumeId: util.StringPtr("stored-pool/stored-image")}

		cfg := runtimeCephConfig()
		cfg.PoolByClass["ssd"] = "runtime-pool"

		pool, image, err := resolveCephDeleteTarget(vol, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(pool).To(Equal("stored-pool"))
		Expect(image).To(Equal("stored-image"))
	})

	It("creates and removes the libvirt ceph secret", func() {
		l, err := virt.NewLibVirtEp("qemu:///system")
		Expect(err).NotTo(HaveOccurred())
		defer l.Close()

		serverID := "server-123"
		Expect(prepareCephSecretForServer(l, serverID)).To(Succeed())
		DeferCleanup(func() {
			_ = removeCephSecretForServer(l, serverID)
		})
		secretUUID := cephSecretUUIDForServer(serverID)
		secret, err := l.Com.LookupSecretByUUIDString(secretUUID)
		Expect(err).NotTo(HaveOccurred())
		Expect(secret).NotTo(BeNil())
		Expect(secret.Free()).To(Succeed())

		Expect(removeCephSecretForServer(l, serverID)).To(Succeed())
		_, err = l.Com.LookupSecretByUUIDString(secretUUID)
		Expect(err).To(HaveOccurred())
	})

	It("rejects empty server id when preparing ceph secret", func() {
		err := prepareCephSecretForServer(nil, "\t")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("server id is required"))
	})

	It("optionally verifies ceph volume create/delete via mactl command", func() {
		if strings.TrimSpace(os.Getenv("MARMOT_CEPH_MACTL_SMOKE")) != "1" {
			Skip("set MARMOT_CEPH_MACTL_SMOKE=1 to enable")
		}

		apiConfig := strings.TrimSpace(os.Getenv("MARMOT_MACTL_API_CONFIG"))
		if apiConfig == "" {
			Skip("set MARMOT_MACTL_API_CONFIG to a logged-in mactl api config file")
		}

		storageClass := strings.TrimSpace(os.Getenv("MARMOT_CEPH_STORAGE_CLASS"))
		if storageClass == "" {
			storageClass = "ssd"
		}

		tempDir := GinkgoT().TempDir()
		mactlBin := filepath.Join(tempDir, "mactl-smoke")
		buildCmd := exec.Command("go", "build", "-o", mactlBin, "../../cmd/mactl")
		buildOut, buildErr := buildCmd.CombinedOutput()
		Expect(buildErr).NotTo(HaveOccurred(), "build mactl failed: %s", string(buildOut))

		volumeName := fmt.Sprintf("ceph-smoke-%d", time.Now().UnixNano())
		volumeYAML := filepath.Join(tempDir, "ceph-volume.yaml")
		yamlContent := fmt.Sprintf("apiVersion: v1\nkind: Volume\nmetadata:\n  name: %s\nspec:\n  kind: data\n  type: ceph\n  size: 1\n  storageClass: %s\n", volumeName, storageClass)
		Expect(os.WriteFile(volumeYAML, []byte(yamlContent), 0o600)).To(Succeed())

		createCmd := exec.Command(mactlBin, "--api", apiConfig, "volume", "create", "-f", volumeYAML, "--output", "json")
		createOut, createErr := createCmd.CombinedOutput()
		Expect(createErr).NotTo(HaveOccurred(), "mactl volume create failed: %s", string(createOut))

		var created api.Volume
		Expect(json.Unmarshal(createOut, &created)).To(Succeed(), "unexpected create output: %s", string(createOut))
		Expect(created.Spec.Type).NotTo(BeNil())
		Expect(strings.ToLower(strings.TrimSpace(*created.Spec.Type))).To(Equal("ceph"))

		createdID := api.VolumeID(created)
		Expect(strings.TrimSpace(createdID)).NotTo(BeEmpty())

		DeferCleanup(func() {
			deleteCmd := exec.Command(mactlBin, "--api", apiConfig, "volume", "delete", createdID, "--output", "json")
			deleteOut, deleteErr := deleteCmd.CombinedOutput()
			Expect(deleteErr).NotTo(HaveOccurred(), "mactl volume delete failed: %s", string(deleteOut))
		})
	})
})
