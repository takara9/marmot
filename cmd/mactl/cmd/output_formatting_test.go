package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
)

var _ = Describe("Output formatting", func() {
	Describe("outputServers", func() {
		It("outputs YAML when output style is yaml", func() {
			original := outputStyle
			outputStyle = "yaml"
			DeferCleanup(func() {
				outputStyle = original
			})

			output := captureOutput(func() {
				err := outputServers([]api.Server{{
					Metadata: api.Metadata{
						Name: "server1",
						Id:   "srv-001",
					},
				}})
				Expect(err).NotTo(HaveOccurred())
			})

			trimmed := strings.TrimSpace(output)
			Expect(trimmed).To(HavePrefix("- "))
			Expect(trimmed).To(ContainSubstring("metadata:"))
			Expect(trimmed).NotTo(ContainSubstring("{\"apiVersion\""))
		})

		It("outputs server list without panicking", func() {
			servers := []api.Server{
				{
					Metadata: api.Metadata{
						Name: "server1",
						Id:   "srv-001",
					},
				},
			}

			Expect(func() {
				captureOutput(func() {
					Expect(outputServers(servers)).NotTo(HaveOccurred())
				})
			}).NotTo(Panic())
		})

		It("outputs empty server list without panicking", func() {
			Expect(func() {
				captureOutput(func() {
					Expect(outputServers([]api.Server{})).NotTo(HaveOccurred())
				})
			}).NotTo(Panic())
		})

		It("includes AGE column in text output", func() {
			createdAt := time.Now().Add(-2 * time.Hour)

			output := captureOutput(func() {
				err := outputServers([]api.Server{{
					Metadata: api.Metadata{Name: "server-age"},
					Status:   &api.Status{CreationTimeStamp: &createdAt},
				}})
				Expect(err).NotTo(HaveOccurred())
			})

			lines := strings.Split(strings.TrimSpace(output), "\n")
			Expect(lines).To(HaveLen(3), output)
			Expect(lines[0]).To(ContainSubstring("AGE"), output)
			Expect(lines[2]).To(ContainSubstring("2h"), output)
		})

		It("hides mgmt continuation line when non-mgmt networks exist by default", func() {
			originalShowAll := getServerShowAll
			getServerShowAll = false
			DeferCleanup(func() {
				getServerShowAll = originalShowAll
			})

			addrPrimary := "172.16.8.3"
			addrSecondary := "192.168.1.64"
			output := captureOutput(func() {
				err := outputServers([]api.Server{{
					Metadata: api.Metadata{Name: "server-na-trim"},
					Spec: api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{
						{Address: &addrPrimary, Networkname: "app-net"},
						{Networkname: "mgmt"},
						{Address: &addrSecondary, Networkname: "host-bridge"},
					}},
				}})
				Expect(err).NotTo(HaveOccurred())
			})

			Expect(output).To(ContainSubstring("app-net"), output)
			Expect(output).To(ContainSubstring("host-bridge"), output)
			Expect(output).NotTo(ContainSubstring("mgmt"), output)
		})

		It("keeps mgmt line when mgmt is the only attached network", func() {
			originalShowAll := getServerShowAll
			getServerShowAll = false
			DeferCleanup(func() {
				getServerShowAll = originalShowAll
			})

			output := captureOutput(func() {
				err := outputServers([]api.Server{{
					Metadata: api.Metadata{Name: "server-mgmt-only"},
					Spec: api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{
						{Networkname: "mgmt"},
					}},
				}})
				Expect(err).NotTo(HaveOccurred())
			})

			Expect(output).To(ContainSubstring("mgmt"), output)
		})

		It("keeps all continuation lines including N/A IP when --all is enabled", func() {
			originalShowAll := getServerShowAll
			getServerShowAll = true
			DeferCleanup(func() {
				getServerShowAll = originalShowAll
			})

			addrPrimary := "172.16.8.3"
			addrSecondary := "192.168.1.64"
			output := captureOutput(func() {
				err := outputServers([]api.Server{{
					Metadata: api.Metadata{Name: "server-na-all"},
					Spec: api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{
						{Address: &addrPrimary, Networkname: "app-net"},
						{Networkname: "mgmt"},
						{Address: &addrSecondary, Networkname: "host-bridge"},
					}},
				}})
				Expect(err).NotTo(HaveOccurred())
			})

			Expect(output).To(ContainSubstring("app-net"), output)
			Expect(output).To(ContainSubstring("mgmt"), output)
			Expect(output).To(ContainSubstring("host-bridge"), output)
		})
	})

	Describe("describeServerText", func() {
		It("outputs server details without panicking", func() {
			server := &api.Server{
				Metadata: api.Metadata{
					Name: "test-server",
					Id:   "srv-001",
				},
			}

			Expect(func() {
				_ = describeServerText(server)
			}).NotTo(Panic())
		})
	})

	Describe("describeNetworkText", func() {
		It("outputs network details without panicking", func() {
			network := &api.VirtualNetwork{
				Metadata: api.Metadata{
					Name: "test-net",
					Id:   "net-001",
				},
			}

			Expect(func() {
				_ = describeNetworkText(network)
			}).NotTo(Panic())
		})
	})

	Describe("describeVolumeText", func() {
		It("outputs volume details without panicking", func() {
			volume := &api.Volume{
				Metadata: api.Metadata{
					Name: "test-vol",
					Id:   "vol-001",
				},
			}

			Expect(func() {
				_ = describeVolumeText(volume)
			}).NotTo(Panic())
		})
	})

	Describe("describeImageText", func() {
		It("outputs image details without panicking", func() {
			image := &api.Image{
				Metadata: api.Metadata{
					Name: "test-img",
					Id:   "img-001",
				},
			}

			Expect(func() {
				_ = describeImageText(image)
			}).NotTo(Panic())
		})
	})

	Describe("describeKubernetesEngineText", func() {
		It("outputs kubernetes engine details without panicking", func() {
			ke := &api.KubernetesEngine{
				Metadata: api.Metadata{
					Name: "test-mke",
					Id:   "mke-001",
				},
				Spec: api.KubernetesEngineSpec{
					Version: "1.36",
					Nodes:   2,
				},
			}

			Expect(func() {
				_ = describeKubernetesEngineText(ke, nil, nil)
			}).NotTo(Panic())
		})

		It("includes node and load balancer rows", func() {
			nodeCPU := 2
			nodeMemory := 4096
			internalAddr := "172.16.1.10"
			externalAddr := "192.168.1.50"
			nodes := []api.Server{{
				Metadata: api.Metadata{Name: "mke-node-1"},
				Spec: api.ServerSpec{
					Cpu:    &nodeCPU,
					Memory: &nodeMemory,
					NetworkInterface: &[]api.NetworkInterface{
						{Networkname: "default", Address: &internalAddr},
						{Networkname: "host-bridge", Address: &externalAddr},
					},
				},
			}}

			output := captureOutput(func() {
				_ = describeKubernetesEngineText(&api.KubernetesEngine{
					Metadata: api.Metadata{Name: "test-mke"},
				}, nodes, nil)
			})

			Expect(output).To(ContainSubstring("mke-node-1"), output)
			Expect(output).To(ContainSubstring(internalAddr), output)
			Expect(output).To(ContainSubstring(externalAddr), output)
		})
	})

	Describe("kubernetesEngineServerIPs", func() {
		It("treats host-bridge as external and the other network as internal", func() {
			internalAddr := "172.16.1.10"
			externalAddr := "192.168.1.50"
			server := api.Server{
				Spec: api.ServerSpec{
					NetworkInterface: &[]api.NetworkInterface{
						{Networkname: "default", Address: &internalAddr},
						{Networkname: "host-bridge", Address: &externalAddr},
					},
				},
			}

			internalIP, externalIP := kubernetesEngineServerIPs(server)
			Expect(internalIP).To(Equal(internalAddr))
			Expect(externalIP).To(Equal(externalAddr))
		})

		It("returns dashes when no network interfaces are set", func() {
			internalIP, externalIP := kubernetesEngineServerIPs(api.Server{})
			Expect(internalIP).To(Equal("-"))
			Expect(externalIP).To(Equal("-"))
		})
	})
})

// captureOutput captures stdout temporarily
func captureOutput(fn func()) string {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		Fail("os.Pipe failed: " + err.Error())
	}
	defer func() {
		_ = r.Close()
	}()

	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	fn()

	if err := w.Close(); err != nil {
		Fail("stdout close failed: " + err.Error())
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		Fail("io.Copy failed: " + err.Error())
	}
	return buf.String()
}
