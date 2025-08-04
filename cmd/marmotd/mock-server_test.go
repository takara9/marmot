package main

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Mock Test", Ordered, func() {
	BeforeAll(func(ctx SpecContext) {
		GinkgoWriter.Println("Start marmot server mock")
		startMockServer()
	}, NodeTimeout(20*time.Second))

	Context("Basic access test", func() {
		time.Sleep(5 * time.Second)
		ep, err := NewMarmotdEp(
			"http",
			"127.0.0.1:8080",
			"/api/v1",
			10)
		if err != nil {
			GinkgoWriter.Println("Error creating MarmotEndpoint:", err)
		} else {
			GinkgoWriter.Println("MarmotEndpoint created successfully:", ep)
		}
		It("Check endpoint", func() {
			GinkgoWriter.Println("MarmotEndpoint created successfully:", ep)
		})
		It("Accessing ping", func() {
			GinkgoWriter.Println("ping marmot-server")
			statusCode, body, url, err := ep.Ping()
			GinkgoWriter.Printf("Status Code: %d, Body: %s, URL: %v, Error: %v\n", statusCode, body, url, err)
			Expect(err).To(BeNil(), "Expected no error from ping")
			Expect(statusCode).To(Equal(200), "Expected status code 200 from ping")
			Expect(string(body)).To(Equal("{\"message\":\"ok\"}\n"), "Expected body to be 'pong'")
			Expect(url).To(BeNil(), "Expected no URL from ping response")
		})
		It("Accessing version", func() {
			GinkgoWriter.Println("get version from marmot-server")
			statusCode, body, url, err := ep.GetVersion()
			GinkgoWriter.Printf("Status Code: %d, Body: %s, URL: %v, Error: %v\n", statusCode, body, url, err)
			Expect(err).To(BeNil(), "Expected no error from ping")
			Expect(statusCode).To(Equal(200), "Expected status code 200 from ping")
			Expect(string(body)).To(Equal("{\"version\":\"0.0.1\"}\n"), "Expected body to be 'pong'")
			Expect(url).To(BeNil(), "Expected no URL from ping response")
		})
	})
})
