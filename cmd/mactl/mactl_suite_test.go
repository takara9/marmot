package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUtil(t *testing.T) {
	if err := ensureMactlTestBinary(); err != nil {
		t.Fatalf("failed to prepare mactl test binary: %v", err)
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "Util Suite")
}
