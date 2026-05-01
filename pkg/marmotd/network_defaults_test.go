package marmotd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("VirtualNetworkDefaults", func() {
	Describe("applyVirtualNetworkDefaults", func() {
		It("defaults overlayMode to vxlan and peerPolicy to auto when omitted", func() {
			network := api.VirtualNetwork{
				Spec: &api.VirtualNetworkSpec{
					Vni: util.IntPtrInt(100),
				},
			}

			err := applyVirtualNetworkDefaults(&network, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(network.Spec.OverlayMode).NotTo(BeNil())
			Expect(*network.Spec.OverlayMode).To(Equal(api.VirtualNetworkSpecOverlayMode(api.Vxlan)))
			Expect(network.Spec.PeerPolicy).NotTo(BeNil())
			Expect(*network.Spec.PeerPolicy).To(Equal(api.VirtualNetworkSpecPeerPolicy(api.Auto)))
		})

		It("does not force peerPolicy for non-vxlan overlays", func() {
			overlayMode := api.None
			network := api.VirtualNetwork{
				Spec: &api.VirtualNetworkSpec{
					OverlayMode: &overlayMode,
				},
			}

			err := applyVirtualNetworkDefaults(&network, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(network.Spec.PeerPolicy).To(BeNil())
		})
	})

	Describe("nextAvailableVNI", func() {
		It("uses the first free VNI from 100", func() {
			networks := []api.VirtualNetwork{
				{Spec: &api.VirtualNetworkSpec{Vni: util.IntPtrInt(100)}},
				{Spec: &api.VirtualNetworkSpec{Vni: util.IntPtrInt(101)}},
				{Spec: &api.VirtualNetworkSpec{Vni: util.IntPtrInt(103)}},
			}

			vni, err := nextAvailableVNI(networks)
			Expect(err).NotTo(HaveOccurred())
			Expect(vni).To(Equal(102))
		})

		It("ignores invalid existing VNI values", func() {
			networks := []api.VirtualNetwork{
				{Spec: &api.VirtualNetworkSpec{Vni: util.IntPtrInt(-1)}},
				{Spec: &api.VirtualNetworkSpec{Vni: util.IntPtrInt(0)}},
			}

			vni, err := nextAvailableVNI(networks)
			Expect(err).NotTo(HaveOccurred())
			Expect(vni).To(Equal(100))
		})
	})
})
