package util

import (
	"encoding/hex"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
)

var _ = Describe("Guest identity generation", Label("unit", "identity"), func() {
	Describe("machineIDForServer", func() {
		It("normalizes Metadata.Uuid into a 32-digit machine-id", func() {
			spec := api.Server{
				Id: "abcde",
				Metadata: &api.Metadata{
					Uuid: StringPtr("550e8400-e29b-41d4-a716-446655440000"),
				},
			}

			Expect(machineIDForServer(spec)).To(Equal("550e8400e29b41d4a716446655440000"))
		})

		It("falls back to a deterministic 32-digit hex value when UUID is absent", func() {
			spec := api.Server{Id: "a123456"}

			got1 := machineIDForServer(spec)
			got2 := machineIDForServer(spec)

			Expect(got1).To(Equal(got2))
			Expect(got1).To(HaveLen(32))
			_, err := hex.DecodeString(got1)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("hostIDBytes", func() {
		It("returns a deterministic 4-byte hostid derived from machine-id", func() {
			a := hostIDBytes("550e8400e29b41d4a716446655440000")
			b := hostIDBytes("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			c := hostIDBytes("550e8400e29b41d4a716446655440000")

			Expect(a).To(HaveLen(4))
			Expect(a).To(Equal(c))
			Expect(a).NotTo(Equal(b))
		})
	})
})
