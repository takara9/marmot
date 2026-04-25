package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	defer r.Close()

	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("stdout close failed: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy failed: %v", err)
	}
	return buf.String()
}

func TestPrintImageDetails_NilSafe(t *testing.T) {
	output := captureStdout(t, func() {
		printImageDetails(api.Image{Id: "img01"})
	})

	if !strings.Contains(output, "Image Details") {
		t.Fatalf("expected header in output, got: %s", output)
	}
	if !strings.Contains(output, "Summary") {
		t.Fatalf("expected summary section in output, got: %s", output)
	}
	if !strings.Contains(output, "Id:           img01") {
		t.Fatalf("expected id in output, got: %s", output)
	}
	if !strings.Contains(output, "UUID:         N/A") {
		t.Fatalf("expected N/A uuid in output, got: %s", output)
	}
	if !strings.Contains(output, "State:        N/A") {
		t.Fatalf("expected N/A state in output, got: %s", output)
	}
}

func TestFormatImageStatus_UnknownFallback(t *testing.T) {
	got := formatImageStatus(&api.Status{StatusCode: 99})
	want := "UNKNOWN(99) (99)"
	if got != want {
		t.Fatalf("unexpected status: got=%q want=%q", got, want)
	}
}
