package marmotd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("Image backing store", func() {
	Describe("CheckImageBackingStore", func() {
		It("passes when all backing stores exist", func() {
			tempDir := GinkgoT().TempDir()
			qcow2Path := filepath.Join(tempDir, "image.qcow2")
			lvPath := filepath.Join(tempDir, "boot-volume")

			Expect(os.WriteFile(qcow2Path, []byte("qcow2"), 0644)).To(Succeed())
			Expect(os.WriteFile(lvPath, []byte("lv"), 0644)).To(Succeed())

			image := api.Image{
				Spec: &api.ImageSpec{
					Qcow2Path: util.StringPtr(qcow2Path),
					LvPath:    util.StringPtr(lvPath),
				},
			}

			Expect(CheckImageBackingStore(image)).To(Succeed())
		})

		It("detects a missing qcow2 file", func() {
			tempDir := GinkgoT().TempDir()
			missingPath := filepath.Join(tempDir, "missing.qcow2")

			image := api.Image{
				Spec: &api.ImageSpec{
					Qcow2Path: util.StringPtr(missingPath),
				},
			}

			err := CheckImageBackingStore(image)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(missingPath))
		})

		It("derives the logical volume path when lvPath is empty", func() {
			image := api.Image{
				Spec: &api.ImageSpec{
					VolumeGroup:   util.StringPtr("vg1"),
					LogicalVolume: util.StringPtr("boot-image"),
				},
			}

			err := CheckImageBackingStore(image)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("/dev/vg1/boot-image"))
		})
	})

	Describe("ensureImageVolumeGroupAvailable", func() {
		var original func(string) error

		BeforeEach(func() {
			original = checkImageVolumeGroup
		})

		AfterEach(func() {
			checkImageVolumeGroup = original
		})

		It("rejects an empty volume group", func() {
			Expect(ensureImageVolumeGroupAvailable("   ")).To(HaveOccurred())
		})

		It("rejects a missing volume group", func() {
			checkImageVolumeGroup = func(vgName string) error {
				return errors.New("not found")
			}

			err := ensureImageVolumeGroupAvailable("vg-missing")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("vg-missing"))
		})

		It("passes when the volume group is available", func() {
			checkImageVolumeGroup = func(vgName string) error {
				if !strings.EqualFold(vgName, "vg-ok") {
					return errors.New("unexpected volume group")
				}
				return nil
			}

			Expect(ensureImageVolumeGroupAvailable("vg-ok")).To(Succeed())
		})
	})
})