package marmotd

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

func TestPreCreatedStorageVolumeReady(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	suiteConfig.LabelFilter = "storage-volume-ready"
	RunSpecs(t, "Pre-created Storage Volume Ready Suite", suiteConfig, reporterConfig)
}

var _ = Describe("事前に作成された Volume の接続可否", Label("storage-volume-ready"), func() {
	volumeWithStatus := func(status int) *api.Volume {
		return &api.Volume{
			Metadata: api.Metadata{Id: "6ba7e", Name: "lvm-vol1"},
			Status:   &api.Status{StatusCode: status},
		}
	}

	DescribeTable("状態ごとの判定",
		func(vol *api.Volume, wantErr bool, wantNotReady bool) {
			err := checkPreCreatedStorageVolumeReady(0, vol)
			if !wantErr {
				Expect(err).NotTo(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrStorageVolumeNotReady)).To(Equal(wantNotReady))
		},
		Entry("存在しない Volume は新規作成に任せる", nil, false, false),
		Entry("状態が未設定の Volume は従来どおり扱う", &api.Volume{Metadata: api.Metadata{Id: "6ba7e"}}, false, false),
		Entry("AVAILABLE は接続できる", volumeWithStatus(db.VOLUME_AVAILABLE), false, false),
		Entry("PENDING は実体の作成待ちとして再試行できる", volumeWithStatus(db.VOLUME_PENDING), true, true),
		Entry("PROVISIONING は実体の作成待ちとして再試行できる", volumeWithStatus(db.VOLUME_PROVISIONING), true, true),
		Entry("ERROR は再試行しない", volumeWithStatus(db.VOLUME_ERROR), true, false),
		Entry("UNAVAILABLE は再試行しない", volumeWithStatus(db.VOLUME_UNAVAILABLE), true, false),
		Entry("DELETING は再試行しない", volumeWithStatus(db.VOLUME_DELETING), true, false),
	)

	It("エラーメッセージに Volume の名前・ID・状態を含める", func() {
		err := checkPreCreatedStorageVolumeReady(1, volumeWithStatus(db.VOLUME_PENDING))
		Expect(err).To(MatchError(ContainSubstring("storage[1]")))
		Expect(err).To(MatchError(ContainSubstring("lvm-vol1")))
		Expect(err).To(MatchError(ContainSubstring("6ba7e")))
		Expect(err).To(MatchError(ContainSubstring("PENDING")))
	})
})
