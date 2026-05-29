package cmd

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
)

var _ = Describe("manifest", func() {
	Describe("normalizeResourceName", func() {
		It("converts plural form to singular", func() {
			Expect(normalizeResourceName("servers")).To(Equal("server"))
			Expect(normalizeResourceName("networks")).To(Equal("network"))
			Expect(normalizeResourceName("volumes")).To(Equal("volume"))
			Expect(normalizeResourceName("images")).To(Equal("image"))
			Expect(normalizeResourceName("gateways")).To(Equal("gateway"))
			Expect(normalizeResourceName("vpngateways")).To(Equal("vpngateway"))
		})

		It("keeps singular form unchanged", func() {
			Expect(normalizeResourceName("server")).To(Equal("server"))
			Expect(normalizeResourceName("network")).To(Equal("network"))
			Expect(normalizeResourceName("volume")).To(Equal("volume"))
			Expect(normalizeResourceName("image")).To(Equal("image"))
			Expect(normalizeResourceName("gateway")).To(Equal("gateway"))
		})

		It("handles aliases", func() {
			Expect(normalizeResourceName("srv")).To(Equal("server"))
			Expect(normalizeResourceName("img")).To(Equal("image"))
			Expect(normalizeResourceName("vol")).To(Equal("volume"))
			Expect(normalizeResourceName("net")).To(Equal("network"))
			Expect(normalizeResourceName("gw")).To(Equal("gateway"))
			Expect(normalizeResourceName("vpngw")).To(Equal("vpngateway"))
		})

		It("returns lowercase for unknown names", func() {
			Expect(normalizeResourceName("UNKNOWN")).To(Equal("unknown"))
			Expect(normalizeResourceName("Pod")).To(Equal("pod"))
		})
	})

	Describe("GetManifestType", func() {
		It("detects server kind", func() {
			result := GetManifestType("Server")
			Expect(result).To(Equal(ManifestTypeServer))
		})

		It("detects network kind", func() {
			result := GetManifestType("VirtualNetwork")
			Expect(result).To(Equal(ManifestTypeNetwork))
		})

		It("detects volume kind", func() {
			result := GetManifestType("Volume")
			Expect(result).To(Equal(ManifestTypeVolume))
		})

		It("detects image kind", func() {
			result := GetManifestType("Image")
			Expect(result).To(Equal(ManifestTypeImage))
		})

		It("detects gateway kind", func() {
			result := GetManifestType("Gateway")
			Expect(result).To(Equal(ManifestTypeGateway))
		})

		It("detects vpn gateway kind", func() {
			result := GetManifestType("VpnGateway")
			Expect(result).To(Equal(ManifestTypeVpnGateway))
		})

		It("returns Unknown for missing kind", func() {
			result := GetManifestType("UnknownKind")
			Expect(result).To(Equal(ManifestTypeUnknown))
		})

		It("is case-insensitive", func() {
			Expect(GetManifestType("server")).To(Equal(ManifestTypeServer))
			Expect(GetManifestType("SERVER")).To(Equal(ManifestTypeServer))
		})
	})

	Describe("LoadManifest", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "manifest-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})

		It("loads YAML manifest from file", func() {
			yamlFile := filepath.Join(tmpDir, "test.yaml")
			content := `apiVersion: v1
kind: Server
metadata:
  name: test-server
spec:
  nodeName: node1
  cpu: 2
  memory: 4096
`
			err := os.WriteFile(yamlFile, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			manifest, err := LoadManifest(yamlFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(manifest["kind"]).To(Equal("Server"))
			Expect(manifest["apiVersion"]).To(Equal("v1"))
		})

		It("loads JSON manifest from file", func() {
			jsonFile := filepath.Join(tmpDir, "test.json")
			content := `{
  "apiVersion": "v1",
  "kind": "Server",
  "metadata": {
    "name": "test-server"
  },
  "spec": {
    "nodeName": "node1"
  }
}`
			err := os.WriteFile(jsonFile, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			manifest, err := LoadManifest(jsonFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(manifest["kind"]).To(Equal("Server"))
		})

		It("returns error for invalid YAML", func() {
			yamlFile := filepath.Join(tmpDir, "invalid.yaml")
			content := `invalid: yaml: content: [[[`
			err := os.WriteFile(yamlFile, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			_, err = LoadManifest(yamlFile)
			Expect(err).To(HaveOccurred())
		})

		It("returns error for non-existent file", func() {
			_, err := LoadManifest(filepath.Join(tmpDir, "nonexistent.yaml"))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ApplyServerDefaults", func() {
		It("works without panicking", func() {
			server := &api.Server{}

			Expect(func() {
				ApplyServerDefaults(server)
			}).NotTo(Panic())
		})

		It("handles nil spec gracefully", func() {
			server := &api.Server{}

			Expect(func() {
				ApplyServerDefaults(server)
			}).NotTo(Panic())
		})
	})
})
