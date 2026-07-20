package ceph_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/ceph"
)

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
		originalConf, hadConf := os.LookupEnv("MARMOT_CEPH_CONF_FILE")
		originalKeyring, hadKeyring := os.LookupEnv("MARMOT_CEPH_KEYRING_FILE")

		dir := GinkgoT().TempDir()
		confPath := filepath.Join(dir, "ceph.conf")
		keyringPath := filepath.Join(dir, "ceph.client.admin.keyring")
		Expect(os.WriteFile(confPath, []byte("[global]\nmon_host = 10.1.4.11:6789\nname = client.admin\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(keyringPath, []byte("[client.admin]\n\tkey = dummy\n"), 0o600)).To(Succeed())
		Expect(os.Setenv("MARMOT_CEPH_CONF_FILE", confPath)).To(Succeed())
		Expect(os.Setenv("MARMOT_CEPH_KEYRING_FILE", keyringPath)).To(Succeed())

		DeferCleanup(func() {
			if hadConf {
				Expect(os.Setenv("MARMOT_CEPH_CONF_FILE", originalConf)).To(Succeed())
			} else {
				Expect(os.Unsetenv("MARMOT_CEPH_CONF_FILE")).To(Succeed())
			}

			if hadKeyring {
				Expect(os.Setenv("MARMOT_CEPH_KEYRING_FILE", originalKeyring)).To(Succeed())
			} else {
				Expect(os.Unsetenv("MARMOT_CEPH_KEYRING_FILE")).To(Succeed())
			}
		})
	})

	Describe("DefaultConfig", func() {
		It("uses ceph conf and keyring files by default", func() {
			cfg := defaultConfigWithCleanup()
			Expect(cfg.ConfFile).NotTo(BeEmpty())
			Expect(cfg.KeyringFile).NotTo(BeEmpty())
			_, err := os.Stat(cfg.ConfFile)
			Expect(err).NotTo(HaveOccurred())
			_, err = os.Stat(cfg.KeyringFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("cleanup is a no-op", func() {
			cfg := ceph.DefaultConfig()
			_, err := os.Stat(cfg.KeyringFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Cleanup()).To(Succeed())
			Expect(cfg.Cleanup()).To(Succeed())
			_, err = os.Stat(cfg.KeyringFile)
			Expect(err).NotTo(HaveOccurred())
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

		It("defaults ceph size when omitted", func() {
			cfg := defaultConfigWithCleanup()
			volume := api.Volume{Spec: api.VolSpec{Type: ptr("ceph"), StorageClass: ptr("ssd")}}
			api.SetVolumeID(&volume, "abcde")

			req, err := ceph.MapVolumeToRequest(volume, cfg)

			Expect(err).NotTo(HaveOccurred())
			Expect(req.Pool).To(Equal("marmot-ssd"))
			Expect(req.SizeGB).To(Equal(1))
			Expect(req.StorageClass).To(Equal("ssd"))
		})

		It("requires storageClass for ceph volumes", func() {
			cfg := defaultConfigWithCleanup()
			volume := api.Volume{Spec: api.VolSpec{Type: ptr("ceph"), Size: intPtr(1)}}
			api.SetVolumeID(&volume, "abcde")

			_, err := ceph.MapVolumeToRequest(volume, cfg)

			Expect(err).To(MatchError("storageClass is required for ceph volumes"))
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
			Expect(runner.commands).To(ContainElement(expectedRBDCommand(cfg, "create", "marmot-ssd/vol-abcde", "--size", "20G")))
		})

		It("parses rbd info JSON", func() {
			command := expectedRBDCommand(cfg, "info", "marmot-ssd/vol-abcde", "--format", "json")
			runner.outputs[command] = []byte(`{"name":"vol-abcde","size":21474836480,"pool":"marmot-ssd"}`)

			info, err := client.StatVolume(context.Background(), "marmot-ssd", "vol-abcde")

			Expect(err).NotTo(HaveOccurred())
			Expect(info.ProviderVolumeID).To(Equal("marmot-ssd/vol-abcde"))
			Expect(info.SizeBytes).To(Equal(uint64(21474836480)))
		})

		It("lists images from json output", func() {
			command := expectedRBDCommand(cfg, "ls", "marmot-ssd", "--format", "json")
			runner.outputs[command] = []byte(`["vol-1","vol-2"]`)

			images, err := client.ListVolumes(context.Background(), "marmot-ssd")

			Expect(err).NotTo(HaveOccurred())
			Expect(images).To(Equal([]string{"vol-1", "vol-2"}))
		})

		It("propagates command errors with context", func() {
			command := expectedRBDCommand(cfg, "rm", "marmot-ssd/vol-abcde")
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

func expectedRBDCommand(cfg ceph.Config, args ...string) string {
	parts := []string{"rbd"}
	if confFile := strings.TrimSpace(cfg.ConfFile); confFile != "" {
		parts = append(parts, "--conf", confFile)
	}
	if keyringFile := strings.TrimSpace(cfg.KeyringFile); keyringFile != "" {
		parts = append(parts, "--keyring", keyringFile)
	}
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}
