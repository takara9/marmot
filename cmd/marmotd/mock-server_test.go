package main

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	//. "github.com/onsi/gomega"
)

var _ = Describe("Mock Test", Ordered, func() {
	BeforeAll(func(ctx SpecContext) {
		GinkgoWriter.Println("Start marmot server mock")
		startMockServer()
	}, NodeTimeout(60*time.Second))

	Context("Test for automatic TSR getting command", func() {
		It("Read User config file", func() {
			GinkgoWriter.Println("Read config")
		})
	})
})
