package marmotd

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHostIDInternal(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	suiteConfig.LabelFilter = "hostid"
	RunSpecs(t, "HostID Internal Suite", suiteConfig, reporterConfig)
}

var _ = Describe("hostid command integration", Label("hostid"), func() {
	var original func() ([]byte, error)

	BeforeEach(func() {
		original = hostIDCommandOutput
	})

	AfterEach(func() {
		hostIDCommandOutput = original
	})

	It("normalizes hostid command output", func() {
		hostIDCommandOutput = func() ([]byte, error) {
			return []byte("0x7f000001\n"), nil
		}

		Expect(getHostID()).To(Equal("7f000001"))
	})

	It("returns empty when hostid command fails", func() {
		hostIDCommandOutput = func() ([]byte, error) {
			return nil, errors.New("command failed")
		}

		Expect(getHostID()).To(BeEmpty())
	})

	It("returns empty for invalid hostid output", func() {
		hostIDCommandOutput = func() ([]byte, error) {
			return []byte("invalid"), nil
		}

		Expect(getHostID()).To(BeEmpty())
	})
})
