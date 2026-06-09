package cmd

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("sanitizeArchiveName", func() {
	It("returns image when sanitized result becomes empty", func() {
		Expect(sanitizeArchiveName("###")).To(Equal("image"))
	})

	It("keeps allowed characters", func() {
		Expect(sanitizeArchiveName("Ubuntu-24.04_LTS")).To(Equal("Ubuntu-24.04_LTS"))
	})
})

var _ = Describe("pickExportableImageByName", func() {
	It("selects latest AVAILABLE image and skips follower images", func() {
		now := time.Now()
		earlier := now.Add(-1 * time.Hour)
		followerLabels := map[string]interface{}{db.ImageLabelSyncRole: "follower"}

		images := []api.Image{
			{
				Metadata: api.Metadata{Name: "ubuntu", Id: "img-follower", Labels: &followerLabels},
				Spec:     api.ImageSpec{Qcow2Path: util.StringPtr("/tmp/follower.qcow2")},
				Status:   &api.Status{StatusCode: db.IMAGE_AVAILABLE, CreationTimeStamp: &now},
			},
			{
				Metadata: api.Metadata{Name: "ubuntu", Id: "img-old"},
				Spec:     api.ImageSpec{Qcow2Path: util.StringPtr("/tmp/old.qcow2")},
				Status:   &api.Status{StatusCode: db.IMAGE_AVAILABLE, CreationTimeStamp: &earlier},
			},
			{
				Metadata: api.Metadata{Name: "ubuntu", Id: "img-new"},
				Spec:     api.ImageSpec{Qcow2Path: util.StringPtr("/tmp/new.qcow2")},
				Status:   &api.Status{StatusCode: db.IMAGE_AVAILABLE, CreationTimeStamp: &now},
			},
		}

		picked, err := pickExportableImageByName(images, "ubuntu")
		Expect(err).NotTo(HaveOccurred())
		Expect(picked.Metadata.Id).To(Equal("img-new"))
	})
})
