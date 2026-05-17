package cmd

import (
	"bytes"
	"io"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
)

var _ = Describe("Output formatting", func() {
	Describe("outputServers", func() {
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
