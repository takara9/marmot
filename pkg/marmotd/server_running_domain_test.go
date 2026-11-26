package marmotd

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/pkg/virt"
)

func TestRunningDomainJudgement(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	suiteConfig.LabelFilter = "running-domain"
	RunSpecs(t, "Running Domain Judgement Suite", suiteConfig, reporterConfig)
}

var _ = Describe("RUNNING サーバーの libvirt 実状態の判定", Label("running-domain"), func() {
	stateOf := func(state virt.DomainState, err error) func(string) (virt.DomainState, error) {
		return func(string) (virt.DomainState, error) { return state, err }
	}

	It("稼働中なら食い違いなしと判定する", func() {
		verdict, err := judgeRunningDomain(stateOf(virt.DomainStateRunning, nil), "vm1")
		Expect(err).NotTo(HaveOccurred())
		Expect(verdict).To(Equal(runningDomainConsistent))
	})

	It("外部操作で停止している場合は外部停止と判定する", func() {
		verdict, err := judgeRunningDomain(stateOf(virt.DomainStateStopped, nil), "vm1")
		Expect(err).NotTo(HaveOccurred())
		Expect(verdict).To(Equal(runningDomainStoppedExternally))
	})

	It("ドメイン定義が存在しない場合は定義消失と判定する", func() {
		verdict, err := judgeRunningDomain(stateOf(virt.DomainStateNotFound, nil), "vm1")
		Expect(err).NotTo(HaveOccurred())
		Expect(verdict).To(Equal(runningDomainMissing))
	})

	It("一時停止などの停止と断定できない状態では何もしない", func() {
		verdict, err := judgeRunningDomain(stateOf(virt.DomainStateOther, nil), "vm1")
		Expect(err).NotTo(HaveOccurred())
		Expect(verdict).To(Equal(runningDomainConsistent))
	})

	It("実状態を取得できない場合はエラーを返し、状態変更の判定をしない", func() {
		verdict, err := judgeRunningDomain(stateOf(virt.DomainStateOther, errors.New("libvirtd unavailable")), "vm1")
		Expect(err).To(HaveOccurred())
		Expect(verdict).To(Equal(runningDomainUnknown))
	})

	It("インスタンス名で実状態を問い合わせる", func() {
		var queried string
		_, err := judgeRunningDomain(func(name string) (virt.DomainState, error) {
			queried = name
			return virt.DomainStateRunning, nil
		}, "vm-instance-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(queried).To(Equal("vm-instance-1"))
	})
})

var _ = Describe("STOPPED サーバーの libvirt 実状態の判定", Label("running-domain"), func() {
	stateOf := func(state virt.DomainState, err error) func(string) (virt.DomainState, error) {
		return func(string) (virt.DomainState, error) { return state, err }
	}

	It("外部操作で稼働している場合は外部起動と判定する", func() {
		verdict, err := judgeStoppedDomain(stateOf(virt.DomainStateRunning, nil), "vm1")
		Expect(err).NotTo(HaveOccurred())
		Expect(verdict).To(Equal(stoppedDomainStartedExternally))
	})

	It("停止している場合は食い違いなしと判定する", func() {
		verdict, err := judgeStoppedDomain(stateOf(virt.DomainStateStopped, nil), "vm1")
		Expect(err).NotTo(HaveOccurred())
		Expect(verdict).To(Equal(stoppedDomainConsistent))
	})

	It("ドメイン定義が存在しない場合は何もしない", func() {
		verdict, err := judgeStoppedDomain(stateOf(virt.DomainStateNotFound, nil), "vm1")
		Expect(err).NotTo(HaveOccurred())
		Expect(verdict).To(Equal(stoppedDomainConsistent))
	})

	It("一時停止などの稼働と断定できない状態では何もしない", func() {
		verdict, err := judgeStoppedDomain(stateOf(virt.DomainStateOther, nil), "vm1")
		Expect(err).NotTo(HaveOccurred())
		Expect(verdict).To(Equal(stoppedDomainConsistent))
	})

	It("実状態を取得できない場合はエラーを返し、状態変更の判定をしない", func() {
		verdict, err := judgeStoppedDomain(stateOf(virt.DomainStateOther, errors.New("libvirtd unavailable")), "vm1")
		Expect(err).To(HaveOccurred())
		Expect(verdict).To(Equal(stoppedDomainUnknown))
	})
})
