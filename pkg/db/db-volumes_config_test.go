package db

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
)

var _ = Describe("VolumeGroupConfig", func() {
	BeforeEach(func() {
		SetDefaultVolumeGroups("sysvg", "datavg")
	})

	AfterEach(func() {
		SetDefaultVolumeGroups(DefaultOSVolumeGroup, DefaultDataVolumeGroup)
	})

	It("OS ボリュームは設定された volume group を使う", func() {
		spec := &api.VolSpec{Kind: strPtr("os")}
		configureLVMVolumeSpec(spec, "abcde")

		Expect(spec.VolumeGroup).NotTo(BeNil())
		Expect(*spec.VolumeGroup).To(Equal("sysvg"))
		Expect(spec.Path).NotTo(BeNil())
		Expect(*spec.Path).To(Equal("/dev/sysvg/oslv-abcde"))
		Expect(spec.LogicalVolume).NotTo(BeNil())
		Expect(*spec.LogicalVolume).To(Equal("oslv-abcde"))
	})

	It("DATA ボリュームは設定された volume group を使う", func() {
		spec := &api.VolSpec{Kind: strPtr("data")}
		configureLVMVolumeSpec(spec, "vwxyz")

		Expect(spec.VolumeGroup).NotTo(BeNil())
		Expect(*spec.VolumeGroup).To(Equal("datavg"))
		Expect(spec.Path).NotTo(BeNil())
		Expect(*spec.Path).To(Equal("/dev/datavg/datalv-vwxyz"))
		Expect(spec.LogicalVolume).NotTo(BeNil())
		Expect(*spec.LogicalVolume).To(Equal("datalv-vwxyz"))
	})
})

func strPtr(value string) *string {
	return &value
}
