package controller

import (
	"github.com/takara9/marmot/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EnsureKubernetesEngineMetricsServer", func() {
	It("returns an error when the control plane status is incomplete", func() {
		ke := api.KubernetesEngine{Metadata: api.Metadata{Name: "demo"}}
		err := EnsureKubernetesEngineMetricsServer(ke)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("control plane status is incomplete"))
	})

	It("returns an error when metrics-server cannot be applied without a running control plane", func() {
		ip := "172.16.90.100"
		port := 6443
		ke := api.KubernetesEngine{
			Metadata: api.Metadata{Name: "demo"},
			Status: &api.Status{
				ControlPlaneIpAddress: &ip,
				ApiServerPort:         &port,
			},
		}
		err := EnsureKubernetesEngineMetricsServer(ke)
		Expect(err).To(HaveOccurred())
	})
})
