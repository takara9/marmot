package virt

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirt"
)

type fakeDestroyer struct {
	state      libvirt.DomainState
	stateErr   error
	destroyErr error
	destroyed  int
}

func (f *fakeDestroyer) GetState() (libvirt.DomainState, int, error) {
	return f.state, 0, f.stateErr
}

func (f *fakeDestroyer) Destroy() error {
	f.destroyed++
	return f.destroyErr
}

var _ = Describe("ドメイン削除時の強制停止", func() {
	DescribeTable("destroyDomainIfActive",
		func(state libvirt.DomainState, wantDestroyed int) {
			f := &fakeDestroyer{state: state}
			Expect(destroyDomainIfActive(f)).To(Succeed())
			Expect(f.destroyed).To(Equal(wantDestroyed))
		},
		Entry("停止中のドメインには Destroy を呼ばない", libvirt.DOMAIN_SHUTOFF, 0),
		Entry("稼働中のドメインは Destroy する", libvirt.DOMAIN_RUNNING, 1),
		Entry("一時停止中のドメインは Destroy する", libvirt.DOMAIN_PAUSED, 1),
		Entry("クラッシュしたドメインは Destroy する", libvirt.DOMAIN_CRASHED, 1),
	)

	It("稼働中のドメインの Destroy が失敗した場合はエラーを返す", func() {
		destroyErr := errors.New("destroy failed")
		f := &fakeDestroyer{state: libvirt.DOMAIN_RUNNING, destroyErr: destroyErr}
		Expect(destroyDomainIfActive(f)).To(MatchError(destroyErr))
	})

	It("状態を取得できない場合は Destroy を呼ばずエラーを返す", func() {
		stateErr := errors.New("get state failed")
		f := &fakeDestroyer{stateErr: stateErr}
		Expect(destroyDomainIfActive(f)).To(MatchError(stateErr))
		Expect(f.destroyed).To(Equal(0))
	})
})
