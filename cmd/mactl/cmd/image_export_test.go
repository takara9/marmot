package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
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

	It("prefers the local node when the same image exists on multiple nodes", func() {
		now := time.Now()
		earier := now.Add(-1 * time.Hour)

		images := []api.Image{
			{
				Metadata: api.Metadata{Name: "server110", Id: "img-remote", NodeName: util.StringPtr("marmot3")},
				Spec:     api.ImageSpec{Qcow2Path: util.StringPtr("/tmp/remote.qcow2")},
				Status:   &api.Status{StatusCode: db.IMAGE_AVAILABLE, CreationTimeStamp: &now},
			},
			{
				Metadata: api.Metadata{Name: "server110", Id: "img-local", NodeName: util.StringPtr("marmot1")},
				Spec:     api.ImageSpec{Qcow2Path: util.StringPtr("/tmp/local.qcow2")},
				Status:   &api.Status{StatusCode: db.IMAGE_AVAILABLE, CreationTimeStamp: &earier},
			},
		}

		picked, err := pickExportableImagesByName(images, "server110", "marmot1")
		Expect(err).NotTo(HaveOccurred())
		Expect(picked).NotTo(BeEmpty())
		Expect(picked[0].Metadata.Id).To(Equal("img-local"))
	})
})

var _ = Describe("writeImageArchive / extractFromTGZ round-trip", func() {
	It("preserves osName and osVersion in the archive", func() {
		image := api.Image{
			Metadata: api.Metadata{Name: "ubuntu22.04"},
			Spec: api.ImageSpec{
				OsName:    util.StringPtr("ubuntu"),
				OsVersion: util.StringPtr("22.04"),
			},
		}
		qcow2Data := []byte("fake-qcow2-data")

		outPath := GinkgoT().TempDir() + "/test.tgz"
		err := writeImageArchive(outPath, image, qcow2Data)
		Expect(err).NotTo(HaveOccurred())

		f, err := os.Open(outPath)
		Expect(err).NotTo(HaveOccurred())
		defer f.Close()

		gz, err := gzip.NewReader(f)
		Expect(err).NotTo(HaveOccurred())
		defer gz.Close()

		tr := tar.NewReader(gz)
		meta := imageArchiveMeta{}
		foundMeta := false
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			Expect(err).NotTo(HaveOccurred())
			if hdr.Name != "metadata.json" {
				continue
			}
			buf, err := io.ReadAll(tr)
			Expect(err).NotTo(HaveOccurred())
			Expect(json.Unmarshal(buf, &meta)).To(Succeed())
			foundMeta = true
			break
		}
		Expect(foundMeta).To(BeTrue())
		Expect(meta.OsName).To(Equal("ubuntu"))
		Expect(meta.OsVersion).To(Equal("22.04"))
	})
})
