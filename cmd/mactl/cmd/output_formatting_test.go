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
					outputServers(servers)
				})
			}).NotTo(Panic())
		})

		It("outputs empty server list without panicking", func() {
			Expect(func() {
				captureOutput(func() {
					outputServers([]api.Server{})
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

		It("keeps continuation lines that have network even when IP is N/A by default", func() {
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
						{Networkname: "default"},
						{Address: &addrSecondary, Networkname: "host-bridge"},
					}},
				}})
				Expect(err).NotTo(HaveOccurred())
			})

			Expect(output).To(ContainSubstring("app-net"), output)
			Expect(output).To(ContainSubstring("host-bridge"), output)
			Expect(output).To(ContainSubstring("default"), output)
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
						{Networkname: "default"},
						{Address: &addrSecondary, Networkname: "host-bridge"},
					}},
				}})
				Expect(err).NotTo(HaveOccurred())
			})

			Expect(output).To(ContainSubstring("app-net"), output)
			Expect(output).To(ContainSubstring("default"), output)
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
})

// captureOutput captures stdout temporarily
func captureOutput(fn func()) string {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		Fail("os.Pipe failed: " + err.Error())
	}
	defer r.Close()

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
