package marmotd

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMarmotdCeph(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Marmotd Cephテストスィート")
}
