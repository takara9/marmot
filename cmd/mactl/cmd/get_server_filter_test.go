package cmd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
)

var _ = Describe("filterVisibleServers", func() {
	It("excludes managedBy-labeled servers by default", func() {
		managedLabels := map[string]interface{}{"managedBy": "gateway-controller"}
		data := []api.Server{
			{Metadata: api.Metadata{Id: "srv-visible", Name: "visible"}},
			{Metadata: api.Metadata{Id: "srv-hidden", Name: "hidden", Labels: &managedLabels}},
		}

		filtered := filterVisibleServers(data, false)

		Expect(filtered).To(HaveLen(1))
		Expect(api.ServerID(filtered[0])).To(Equal("srv-visible"))
	})

	It("keeps managedBy-labeled servers when showAll is true", func() {
		managedLabels := map[string]interface{}{"managedBy": "gateway-controller"}
		data := []api.Server{
			{Metadata: api.Metadata{Id: "srv-visible", Name: "visible"}},
			{Metadata: api.Metadata{Id: "srv-hidden", Name: "hidden", Labels: &managedLabels}},
		}

		all := filterVisibleServers(data, true)

		Expect(all).To(HaveLen(2))
	})

	It("treats managedBy key presence as managed even when empty", func() {
		emptyManagedLabels := map[string]interface{}{"managedBy": ""}
		data := []api.Server{
			{Metadata: api.Metadata{Id: "srv-empty-managed", Labels: &emptyManagedLabels}},
		}

		filtered := filterVisibleServers(data, false)

		Expect(filtered).To(BeEmpty())
	})
})
