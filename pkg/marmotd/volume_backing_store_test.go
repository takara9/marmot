package marmotd

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("Volume backing store", func() {
	Describe("CheckVolumeBackingStore", func() {
		It("passes when a qcow2 file exists", func() {
			tempDir := GinkgoT().TempDir()
			qcow2Path := filepath.Join(tempDir, "disk.qcow2")

			Expect(os.WriteFile(qcow2Path, []byte("qcow2"), 0644)).To(Succeed())

			volume := api.Volume{
				Spec: &api.VolSpec{
					Type: util.StringPtr("qcow2"),
					Path: util.StringPtr(qcow2Path),
				},
			}

			Expect(CheckVolumeBackingStore(volume)).To(Succeed())
		})

		It("detects a missing qcow2 file", func() {
			missingPath := filepath.Join(GinkgoT().TempDir(), "missing.qcow2")

			volume := api.Volume{
				Spec: &api.VolSpec{
					Type: util.StringPtr("qcow2"),
					Path: util.StringPtr(missingPath),
				},
			}

			err := CheckVolumeBackingStore(volume)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(missingPath))
		})

		It("derives the logical volume path when path is empty", func() {
			volume := api.Volume{
				Spec: &api.VolSpec{
					Type:          util.StringPtr("lvm"),
					VolumeGroup:   util.StringPtr("vg1"),
					LogicalVolume: util.StringPtr("oslv-demo"),
				},
			}

			err := CheckVolumeBackingStore(volume)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("/dev/vg1/oslv-demo"))
		})
	})
})