package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMaadm(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Admin Tool")
}
