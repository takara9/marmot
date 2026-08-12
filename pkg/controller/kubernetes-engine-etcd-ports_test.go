package controller

import (
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("parsePortFromEndpoint", func() {
	DescribeTable("parses the port from an endpoint string",
		func(endpoint string, wantPort int, wantOk bool) {
			port, ok := parsePortFromEndpoint(endpoint)
			Expect(ok).To(Equal(wantOk))
			Expect(port).To(Equal(wantPort))
		},
		Entry("http url", "http://127.0.0.1:2379", 2379, true),
		Entry("host port", "127.0.0.1:2380", 2380, true),
		Entry("empty", "", 0, false),
		Entry("no port", "127.0.0.1", 0, false),
	)
})

var _ = Describe("AllocateKubernetesEngineEtcdPorts", func() {
	It("avoids its own etcd ports and other clusters' allocated ports", func() {
		rangeStart, rangeEnd := 40000, 40010

		existing := api.KubernetesEngine{
			Status: &api.Status{
				EtcdClientPort: util.IntPtrInt(40000),
				EtcdPeerPort:   util.IntPtrInt(40001),
			},
		}

		clientPort, peerPort, err := AllocateKubernetesEngineEtcdPorts([]api.KubernetesEngine{existing}, "http://127.0.0.1:40002", rangeStart, rangeEnd)
		Expect(err).NotTo(HaveOccurred())
		Expect(clientPort).NotTo(Equal(40000))
		Expect(clientPort).NotTo(Equal(40001))
		Expect(clientPort).NotTo(Equal(40002))
		Expect(peerPort).NotTo(Equal(40002))
		Expect(peerPort).To(Equal(clientPort + 1))
	})

	It("skips ports that are not locally free", func() {
		rangeStart, rangeEnd := 41000, 41010

		origProbe := kubernetesEnginePortProbe
		DeferCleanup(func() { kubernetesEnginePortProbe = origProbe })
		kubernetesEnginePortProbe = func(port int) bool {
			// 最初の候補ペアだけ使用中(bind不可)を模擬する。
			return port != 41000 && port != 41001
		}

		clientPort, peerPort, err := AllocateKubernetesEngineEtcdPorts(nil, "", rangeStart, rangeEnd)
		Expect(err).NotTo(HaveOccurred())
		Expect(clientPort).To(Equal(41002))
		Expect(peerPort).To(Equal(41003))
	})

	It("returns an error when no free port pair is available", func() {
		origProbe := kubernetesEnginePortProbe
		DeferCleanup(func() { kubernetesEnginePortProbe = origProbe })
		kubernetesEnginePortProbe = func(int) bool { return false }

		_, _, err := AllocateKubernetesEngineEtcdPorts(nil, "", 42000, 42004)
		Expect(err).To(HaveOccurred())
	})
})
