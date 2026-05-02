package marmotd_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

// テスト用ヘルパー: HostStatus を生成する
func newHostStatus(nodeName, hostID string, totalVMs int, updatedSecondsAgo int) api.HostStatus {
	t := time.Now().Add(-time.Duration(updatedSecondsAgo) * time.Second)
	return api.HostStatus{
		NodeName:    util.StringPtr(nodeName),
		HostId:      util.StringPtr(hostID),
		LastUpdated: &t,
		Allocation: &api.HostAllocation{
			TotalVMs: util.IntPtrInt(totalVMs),
		},
	}
}

var _ = Describe("スケジューラー", func() {

	Describe("IsSchedulerLeader", func() {
		Context("アクティブなホストが複数存在する場合", func() {
			var statuses []api.HostStatus

			BeforeEach(func() {
				statuses = []api.HostStatus{
					newHostStatus("hv3", "00000030", 0, 5),
					newHostStatus("hv1", "00000010", 2, 5),
					newHostStatus("hv2", "00000020", 1, 5),
				}
			})

			It("hostId が最小のノードがリーダーになる", func() {
				Expect(marmotd.IsSchedulerLeader("hv1", statuses)).To(BeTrue())
				Expect(marmotd.IsSchedulerLeader("hv2", statuses)).To(BeFalse())
				Expect(marmotd.IsSchedulerLeader("hv3", statuses)).To(BeFalse())
			})
		})

		Context("hostId が同値の場合", func() {
			It("NodeName 辞書順で先頭のノードがリーダーになる", func() {
				statuses := []api.HostStatus{
					newHostStatus("hv2", "00000010", 1, 5),
					newHostStatus("hv1", "00000010", 1, 5),
				}
				Expect(marmotd.IsSchedulerLeader("hv1", statuses)).To(BeTrue())
				Expect(marmotd.IsSchedulerLeader("hv2", statuses)).To(BeFalse())
			})
		})

		Context("一部のホストが非アクティブな場合", func() {
			It("期限切れホストをリーダー候補から除外する", func() {
				statuses := []api.HostStatus{
					newHostStatus("hv1", "00000001", 0, 60), // 60秒前 → 非アクティブ
					newHostStatus("hv2", "000000ff", 0, 5),  // 5秒前  → アクティブ
				}
				// hv1 は非アクティブなので hv2 がリーダー
				Expect(marmotd.IsSchedulerLeader("hv1", statuses)).To(BeFalse())
				Expect(marmotd.IsSchedulerLeader("hv2", statuses)).To(BeTrue())
			})
		})

		Context("アクティブなホストが存在しない場合", func() {
			It("false を返す", func() {
				statuses := []api.HostStatus{
					newHostStatus("hv1", "00000001", 0, 60),
				}
				Expect(marmotd.IsSchedulerLeader("hv1", statuses)).To(BeFalse())
			})
		})

		Context("HostStatus が空の場合", func() {
			It("false を返す", func() {
				Expect(marmotd.IsSchedulerLeader("hv1", nil)).To(BeFalse())
			})
		})

		Context("自ノードが存在しない場合", func() {
			It("false を返す", func() {
				statuses := []api.HostStatus{
					newHostStatus("hv1", "00000001", 0, 5),
				}
				Expect(marmotd.IsSchedulerLeader("hv9", statuses)).To(BeFalse())
			})
		})

		Context("hostId が未設定の場合", func() {
			It("false を返す", func() {
				t := time.Now().Add(-5 * time.Second)
				statuses := []api.HostStatus{{NodeName: util.StringPtr("hv1"), LastUpdated: &t}}
				Expect(marmotd.IsSchedulerLeader("hv1", statuses)).To(BeFalse())
			})
		})
	})

	Describe("SelectNode", func() {
		Context("割り当て済みVM数(TotalVMs) が異なる複数のアクティブノードがある場合", func() {
			It("TotalVMs が最小のノードを選択する", func() {
				statuses := []api.HostStatus{
					newHostStatus("hv1", "00000001", 5, 5),
					newHostStatus("hv2", "00000002", 2, 5),
					newHostStatus("hv3", "00000003", 8, 5),
				}
				node, err := marmotd.SelectNode(statuses)
				Expect(err).NotTo(HaveOccurred())
				Expect(node).To(Equal("hv2"))
			})
		})

		Context("TotalVMs が同点の場合", func() {
			It("NodeName 辞書順で先頭のノードを選択する（決定的）", func() {
				statuses := []api.HostStatus{
					newHostStatus("hv3", "00000003", 3, 5),
					newHostStatus("hv1", "00000001", 3, 5),
					newHostStatus("hv2", "00000002", 3, 5),
				}
				node, err := marmotd.SelectNode(statuses)
				Expect(err).NotTo(HaveOccurred())
				Expect(node).To(Equal("hv1"))
			})
		})

		Context("Allocation が未設定のノードがある場合", func() {
			It("TotalVMs = 0 として扱い選択する", func() {
				t := time.Now().Add(-5 * time.Second)
				statuses := []api.HostStatus{
					{NodeName: util.StringPtr("hv1"), HostId: util.StringPtr("00000001"), LastUpdated: &t, Allocation: nil},
					newHostStatus("hv2", "00000002", 3, 5),
				}
				node, err := marmotd.SelectNode(statuses)
				Expect(err).NotTo(HaveOccurred())
				Expect(node).To(Equal("hv1"))
			})
		})

		Context("非アクティブなホストのみの場合", func() {
			It("ErrNoActiveHosts を返す", func() {
				statuses := []api.HostStatus{
					newHostStatus("hv1", "00000001", 0, 60),
				}
				_, err := marmotd.SelectNode(statuses)
				Expect(err).To(MatchError(marmotd.ErrNoActiveHosts))
			})
		})

		Context("HostStatus が空の場合", func() {
			It("ErrNoActiveHosts を返す", func() {
				_, err := marmotd.SelectNode(nil)
				Expect(err).To(MatchError(marmotd.ErrNoActiveHosts))
			})
		})
	})
})
