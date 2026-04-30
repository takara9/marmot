package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
)

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		Fail("os.Pipe failed: " + err.Error())
	}
	defer r.Close()

	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	fn()

	if err := w.Close(); err != nil {
		Fail("stdout close failed: " + err.Error())
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		Fail("io.Copy failed: " + err.Error())
	}
	return buf.String()
}

var _ = Describe("printImageDetails", func() {
	It("prints a nil-safe detail view", func() {
		output := captureStdout(func() {
			printImageDetails(api.Image{Id: "img01"})
		})

		Expect(strings.Contains(output, "Image Details")).To(BeTrue(), output)
		Expect(strings.Contains(output, "Summary")).To(BeTrue(), output)
		Expect(strings.Contains(output, "Id:           img01")).To(BeTrue(), output)
		Expect(strings.Contains(output, "UUID:         N/A")).To(BeTrue(), output)
		Expect(strings.Contains(output, "State:        N/A")).To(BeTrue(), output)
	})
})

var _ = Describe("formatImageStatus", func() {
	It("falls back for unknown status codes", func() {
		Expect(formatImageStatus(&api.Status{StatusCode: 99})).To(Equal("UNKNOWN(99) (99)"))
	})
})
