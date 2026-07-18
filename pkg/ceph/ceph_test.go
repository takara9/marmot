package ceph_test

import (
	"context"
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/ceph"
)

const testCephMonitorHost = "ceph-mon.example"
const testKeyringPath = "testdata/ceph.client.ubuntu.keyring"

type stubRunner struct {
	commands []string
	outputs  map[string][]byte
	errors   map[string]error
}

func (s *stubRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := name
	for _, arg := range args {
		command += " " + arg
	}
	s.commands = append(s.commands, command)
	if err, ok := s.errors[command]; ok {
		return s.outputs[command], ceph.CommandError{Command: command, Output: string(s.outputs[command]), Err: err}
	}
	if output, ok := s.outputs[command]; ok {
		return output, nil
	}
	return nil, nil
}

var _ = Describe("Ceph", func() {
	BeforeEach(func() {
		originalIP, hadIP := os.LookupEnv("CEPH_IPADDR")
		originalKey, hadKey := os.LookupEnv("CEPH_POOL_KEY")

		Expect(os.Setenv("CEPH_IPADDR", testCephMonitorHost)).To(Succeed())
		// CI は環境変数優先、ローカルは testdata の keyring を読み込む
		if !hadKey || strings.TrimSpace(originalKey) == "" {
			if os.Getenv("GITHUB_ACTIONS") == "true" || strings.EqualFold(os.Getenv("CI"), "true") {
				Fail("CEPH_POOL_KEY must be set in CI/GitHub Actions")
			}
			keyring, err := os.ReadFile(testKeyringPath)
			if err != nil {
				if !os.IsNotExist(err) {
					Expect(err).NotTo(HaveOccurred())
				}
				keyring = []byte("[client.ubuntu]\n\tkey = dummy\n")
			}
			Expect(os.Setenv("CEPH_POOL_KEY", string(keyring))).To(Succeed())
		}
		DeferCleanup(func() {
			if hadIP {
				Expect(os.Setenv("CEPH_IPADDR", originalIP)).To(Succeed())
			} else {
				Expect(os.Unsetenv("CEPH_IPADDR")).To(Succeed())
			}

			if hadKey {
				Expect(os.Setenv("CEPH_POOL_KEY", originalKey)).To(Succeed())
			} else {
				Expect(os.Unsetenv("CEPH_POOL_KEY")).To(Succeed())
			}

			// Remove any temp keyring files created from CEPH_POOL_KEY during tests.
			tempDir := os.TempDir()
			entries, err := os.ReadDir(tempDir)
			if err == nil {
				for _, entry := range entries {
					name := entry.Name()
					if strings.HasPrefix(name, "marmot-ceph-pool-") && strings.HasSuffix(name, ".keyring") {
						_ = os.Remove(tempDir + string(os.PathSeparator) + name)
					}
				}
			}
		})
	})

	Describe("DefaultConfig", func() {
		It("uses the requested monitor host by default", func() {
			cfg := defaultConfigWithCleanup()
			Expect(cfg.Monitors).To(ContainElement(testCephMonitorHost))
			Expect(cfg.MonitorHosts()).To(Equal(testCephMonitorHost))
			Expect(cfg.KeyFile).NotTo(BeEmpty())
			writtenKeyring, err := os.ReadFile(cfg.KeyFile)
			Expect(err).NotTo(HaveOccurred())
			expectedKeyring := strings.TrimSpace(os.Getenv("CEPH_POOL_KEY"))
			if !strings.HasSuffix(expectedKeyring, "\n") {
				expectedKeyring += "\n"
			}
			Expect(string(writtenKeyring)).To(Equal(expectedKeyring))
		})

		It("cleans up the generated keyring on demand", func() {
			cfg := ceph.DefaultConfig()
			Expect(cfg.KeyFile).NotTo(BeEmpty())
			_, err := os.Stat(cfg.KeyFile)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.Cleanup()).To(Succeed())
			Expect(cfg.Cleanup()).To(Succeed())
			_, err = os.Stat(cfg.KeyFile)
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
	})

	Describe("MapVolumeToRequest", func() {
		It("maps a ceph volume into an RBD create request", func() {
			cfg := defaultConfigWithCleanup()
			volume := api.Volume{
				Spec: api.VolSpec{
					Type:         ptr("ceph"),
					Kind:         ptr("data"),
					Size:         intPtr(20),
					StorageClass: ptr(" ssd "),
				},
			}
			api.SetVolumeID(&volume, "abcde")

			req, err := ceph.MapVolumeToRequest(volume, cfg)

			Expect(err).NotTo(HaveOccurred())
			Expect(req.Pool).To(Equal("marmot-ssd"))
			Expect(req.Image).To(Equal("vol-abcde"))
			Expect(req.SizeGB).To(Equal(20))
			Expect(req.ProviderVolumeID()).To(Equal("marmot-ssd/vol-abcde"))
		})

		It("defaults ceph size and storage class when omitted", func() {
			cfg := defaultConfigWithCleanup()
			volume := api.Volume{Spec: api.VolSpec{Type: ptr("ceph")}}
			api.SetVolumeID(&volume, "abcde")

			req, err := ceph.MapVolumeToRequest(volume, cfg)

			Expect(err).NotTo(HaveOccurred())
			Expect(req.Pool).To(Equal("marmot-ssd"))
			Expect(req.SizeGB).To(Equal(1))
			Expect(req.StorageClass).To(Equal("ssd"))
		})

		It("rejects unsupported storage classes", func() {
			cfg := defaultConfigWithCleanup()
			volume := api.Volume{Spec: api.VolSpec{Type: ptr("ceph"), Size: intPtr(1), StorageClass: ptr("sas")}}
			api.SetVolumeID(&volume, "abcde")

			_, err := ceph.MapVolumeToRequest(volume, cfg)

			Expect(err).To(MatchError("storageClass must be one of hdd, ssd, nvme"))
		})
	})

	Describe("Keyring", func() {
		It("loads the moved keyring from testdata", func() {
			if _, err := os.Stat(testKeyringPath); os.IsNotExist(err) {
				Skip("keyring fixture not present (CI uses CEPH_POOL_KEY secret directly)")
			}
			data, err := os.ReadFile(testKeyringPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).NotTo(BeEmpty())
		})
	})

	Describe("Client", func() {
		var runner *stubRunner
		var client *ceph.Client
		var cfg ceph.Config

		BeforeEach(func() {
			runner = &stubRunner{outputs: map[string][]byte{}, errors: map[string]error{}}
			cfg = defaultConfigWithCleanup()
			var err error
			client, err = ceph.NewClient(cfg, runner)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				Expect(client.Cleanup()).To(Succeed())
			})
		})

		It("issues an rbd create command", func() {
			err := client.CreateVolume(context.Background(), ceph.VolumeRequest{Pool: "marmot-ssd", Image: "vol-abcde", SizeGB: 20})

			Expect(err).NotTo(HaveOccurred())
			Expect(runner.commands).To(ContainElement(expectedRBDCommand(cfg, testCephMonitorHost, "create", "marmot-ssd/vol-abcde", "--size", "20G")))
		})

		It("parses rbd info JSON", func() {
			command := expectedRBDCommand(cfg, testCephMonitorHost, "info", "marmot-ssd/vol-abcde", "--format", "json")
			runner.outputs[command] = []byte(`{"name":"vol-abcde","size":21474836480,"pool":"marmot-ssd"}`)

			info, err := client.StatVolume(context.Background(), "marmot-ssd", "vol-abcde")

			Expect(err).NotTo(HaveOccurred())
			Expect(info.ProviderVolumeID).To(Equal("marmot-ssd/vol-abcde"))
			Expect(info.SizeBytes).To(Equal(uint64(21474836480)))
		})

		It("lists images from json output", func() {
			command := expectedRBDCommand(cfg, testCephMonitorHost, "ls", "marmot-ssd", "--format", "json")
			runner.outputs[command] = []byte(`["vol-1","vol-2"]`)

			images, err := client.ListVolumes(context.Background(), "marmot-ssd")

			Expect(err).NotTo(HaveOccurred())
			Expect(images).To(Equal([]string{"vol-1", "vol-2"}))
		})

		It("propagates command errors with context", func() {
			command := expectedRBDCommand(cfg, testCephMonitorHost, "rm", "marmot-ssd/vol-abcde")
			runner.errors[command] = fmt.Errorf("exit status 1")
			runner.outputs[command] = []byte("permission denied")

			err := client.DeleteVolume(context.Background(), "marmot-ssd", "vol-abcde")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("permission denied"))
		})
	})

	Describe("FakeClient", func() {
		It("records operations for higher-level tests", func() {
			fake := &ceph.FakeClient{}

			Expect(fake.CreateVolume(context.Background(), ceph.VolumeRequest{Pool: "marmot-ssd", Image: "vol-abcde", SizeGB: 1})).To(Succeed())
			_, err := fake.StatVolume(context.Background(), "marmot-ssd", "vol-abcde")
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.Created).To(HaveLen(1))
			Expect(fake.Stated).To(Equal([]string{"marmot-ssd/vol-abcde"}))
		})
	})
})

func ptr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func defaultConfigWithCleanup() ceph.Config {
	cfg := ceph.DefaultConfig()
	DeferCleanup(func() {
		Expect(cfg.Cleanup()).To(Succeed())
	})
	return cfg
}

func expectedRBDCommand(cfg ceph.Config, monitor string, args ...string) string {
	parts := []string{"rbd"}
	if user := strings.TrimSpace(cfg.User); user != "" {
		parts = append(parts, "--id", user)
	}
	if keyFile := strings.TrimSpace(cfg.KeyFile); keyFile != "" {
		parts = append(parts, "--keyring", keyFile)
	}
	parts = append(parts, "-m", monitor)
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}
