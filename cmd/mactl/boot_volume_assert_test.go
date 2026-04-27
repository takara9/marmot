package main

import (
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
)

const expectedMockServerNodeName = "hvc"

func expectServerBootVolumeNodeName(g Gomega, server api.Server) {
	g.Expect(server.Spec).NotTo(BeNil())
	g.Expect(server.Spec.BootVolume).NotTo(BeNil())
	g.Expect(server.Spec.BootVolume.Metadata).NotTo(BeNil())
	g.Expect(server.Spec.BootVolume.Metadata.NodeName).NotTo(BeNil())
	g.Expect(*server.Spec.BootVolume.Metadata.NodeName).To(Equal(expectedMockServerNodeName))
}
