package marmotd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("VirtualNetworkDefaults", func() {
	Describe("applyVirtualNetworkDefaults", func() {
		It("defaults overlayMode to geneve when omitted", func() {
			network := api.VirtualNetwork{
				Spec: api.VirtualNetworkSpec{},
			}

			err := applyVirtualNetworkDefaults(&network, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(network.Spec.OverlayMode).NotTo(BeNil())
			Expect(*network.Spec.OverlayMode).To(Equal(api.VirtualNetworkSpecOverlayMode(api.Geneve)))
			Expect(network.Spec.PeerPolicy).To(BeNil())
		})

		It("defaults peerPolicy to auto for vxlan overlays", func() {
			overlayMode := api.Vxlan
			network := api.VirtualNetwork{
				Spec: api.VirtualNetworkSpec{
					OverlayMode: &overlayMode,
				},
			}

			err := applyVirtualNetworkDefaults(&network, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(network.Spec.PeerPolicy).NotTo(BeNil())
			Expect(*network.Spec.PeerPolicy).To(Equal(api.VirtualNetworkSpecPeerPolicy(api.Auto)))
		})

		It("does not force peerPolicy for non-vxlan overlays", func() {
			overlayMode := api.None
			network := api.VirtualNetwork{
				Spec: api.VirtualNetworkSpec{
					OverlayMode: &overlayMode,
				},
			}

			err := applyVirtualNetworkDefaults(&network, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(network.Spec.PeerPolicy).To(BeNil())
		})

		It("does not auto-assign vni when omitted", func() {
			overlayMode := api.Vxlan
			network := api.VirtualNetwork{
				Spec: api.VirtualNetworkSpec{
					OverlayMode: &overlayMode,
				},
			}

			err := applyVirtualNetworkDefaults(&network, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(network.Spec.Vni).To(BeNil())
		})

		It("returns error for out-of-range vni", func() {
			overlayMode := api.Vxlan
			network := api.VirtualNetwork{
				Spec: api.VirtualNetworkSpec{
					OverlayMode: &overlayMode,
					Vni:         util.IntPtrInt(maxVNI + 1),
				},
			}

			err := applyVirtualNetworkDefaults(&network, nil, nil)
			Expect(err).To(HaveOccurred())
		})
	})
})
