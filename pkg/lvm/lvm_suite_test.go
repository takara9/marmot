package lvm

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLvm(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lvm Suite")
}
