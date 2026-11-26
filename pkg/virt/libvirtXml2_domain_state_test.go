package virt

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirt"
)

var _ = Describe("libvirt のドメイン状態の分類", func() {
	DescribeTable("domainStateFromLibvirt",
		func(state libvirt.DomainState, want DomainState) {
			Expect(domainStateFromLibvirt(state)).To(Equal(want))
		},
		Entry("RUNNING は稼働中", libvirt.DOMAIN_RUNNING, DomainStateRunning),
		Entry("BLOCKED は稼働中", libvirt.DOMAIN_BLOCKED, DomainStateRunning),
		Entry("SHUTOFF は停止", libvirt.DOMAIN_SHUTOFF, DomainStateStopped),
		Entry("PAUSED は停止と断定しない", libvirt.DOMAIN_PAUSED, DomainStateOther),
		Entry("SHUTDOWN(シャットダウン中)は停止と断定しない", libvirt.DOMAIN_SHUTDOWN, DomainStateOther),
		Entry("CRASHED は停止と断定しない", libvirt.DOMAIN_CRASHED, DomainStateOther),
		Entry("PMSUSPENDED は停止と断定しない", libvirt.DOMAIN_PMSUSPENDED, DomainStateOther),
		Entry("NOSTATE は停止と断定しない", libvirt.DOMAIN_NOSTATE, DomainStateOther),
	)
})
