package cmd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
)

var _ = Describe("kubectl-like commands", Ordered, func() {
	Describe("resource name handling", func() {
		It("normalizes plural resource names", func() {
			testCases := map[string]string{
				"servers":  "server",
				"images":   "image",
				"networks": "network",
				"volumes":  "volume",
				"gateways": "gateway",
			}

			for input, expected := range testCases {
				result := normalizeResourceName(input)
				Expect(result).To(Equal(expected), "failed for input: "+input)
			}
		})

		It("recognizes resource aliases", func() {
			testCases := map[string]string{
				"srv": "server",
				"img": "image",
				"vol": "volume",
				"net": "network",
				"gw":  "gateway",
			}

			for input, expected := range testCases {
				result := normalizeResourceName(input)
				Expect(result).To(Equal(expected), "failed for input: "+input)
			}
		})

		It("returns lowercase for unknown names", func() {
			result := normalizeResourceName("server")
			Expect(result).To(Equal("server"))

			result = normalizeResourceName("network")
			Expect(result).To(Equal("network"))
		})
	})

	Describe("GetKindFromResourceName", func() {
		It("converts resource name to Kind", func() {
			Expect(GetKindFromResourceName("server")).To(Equal("Server"))
			Expect(GetKindFromResourceName("image")).To(Equal("Image"))
			Expect(GetKindFromResourceName("network")).To(Equal("VirtualNetwork"))
			Expect(GetKindFromResourceName("volume")).To(Equal("Volume"))
			Expect(GetKindFromResourceName("gateway")).To(Equal("Gateway"))
		})

		It("handles aliases", func() {
			Expect(GetKindFromResourceName("srv")).To(Equal("Server"))
			Expect(GetKindFromResourceName("img")).To(Equal("Image"))
		})

		It("handles plural forms", func() {
			Expect(GetKindFromResourceName("servers")).To(Equal("Server"))
			Expect(GetKindFromResourceName("images")).To(Equal("Image"))
		})

		It("returns empty string for unknown resource", func() {
			Expect(GetKindFromResourceName("unknown")).To(Equal(""))
		})
	})

	Describe("ApplyServerDefaults", func() {
		It("handles nil server gracefully", func() {
			Expect(func() {
				ApplyServerDefaults(nil)
			}).NotTo(Panic())
		})

		It("does not panic with empty server", func() {
			var server *api.Server
			Expect(func() {
				ApplyServerDefaults(server)
			}).NotTo(Panic())
		})
	})
})
