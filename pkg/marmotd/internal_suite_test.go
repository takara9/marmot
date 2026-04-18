package marmotd

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMarmotdInternal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Marmotd Internal Test Suite")
}