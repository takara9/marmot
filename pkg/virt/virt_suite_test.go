package virt_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVirt(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Virt Suite")
}
