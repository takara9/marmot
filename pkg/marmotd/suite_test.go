package marmotd_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMarmotd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Marmotd Test Suite")
}

var _ = BeforeSuite(func() {
	fmt.Println("Preparing...")
	prepareMockVmfunc()
	prepareMockServers()
	fmt.Println("Finish preparing!")
})

var _ = AfterSuite(func() {
	fmt.Println("Clean up...")
	cleanupMockVmfunc()
	cleanupMockServers()
	fmt.Println("Finish clean up!")
})

var _ = Describe("Test marmot server functions", func() {
	Context("internal functions of Marmot servers", testMarmotFuncs)
	Context("api calles for Marmot servers", testMarmotd)
})
