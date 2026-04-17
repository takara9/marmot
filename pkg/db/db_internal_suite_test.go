package db

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDbInternal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Db Internal Suite")
}
