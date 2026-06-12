package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestExtractResponseIDPrefersMetadataID(t *testing.T) {
	id, err := extractResponseID([]byte(`{"id":"legacy","metadata":{"id":"meta-123"}}`))
	if err != nil {
		t.Fatalf("extractResponseID() unexpected err: %v", err)
	}
	if id != "meta-123" {
		t.Fatalf("extractResponseID() = %q, want meta-123", id)
	}
}

func TestExtractResponseIDFallbackTopLevelID(t *testing.T) {
	id, err := extractResponseID([]byte(`{"id":"top-999"}`))
	if err != nil {
		t.Fatalf("extractResponseID() unexpected err: %v", err)
	}
	if id != "top-999" {
		t.Fatalf("extractResponseID() = %q, want top-999", id)
	}
}

func TestExtractResponseIDEmptyWhenMissing(t *testing.T) {
	id, err := extractResponseID([]byte(`{"metadata":{}}`))
	if err != nil {
		t.Fatalf("extractResponseID() unexpected err: %v", err)
	}
	if id != "" {
		t.Fatalf("extractResponseID() = %q, want empty", id)
	}
}

func TestProcessCreateResponseUsesMetadataIDInText(t *testing.T) {
	withOutputStyle("text", t, func() {
		out := captureStdoutForIDTest(t, func() {
			err := processCreateResponse([]byte(`{"id":null,"metadata":{"id":"vol-101"}}`))
			if err != nil {
				t.Fatalf("processCreateResponse() unexpected err: %v", err)
			}
		})

		if !strings.Contains(out, "ID: vol-101") {
			t.Fatalf("stdout = %q, want contains ID: vol-101", out)
		}
	})
}

func TestProcessApplyResponseUsesMetadataIDInText(t *testing.T) {
	withOutputStyle("text", t, func() {
		out := captureStdoutForIDTest(t, func() {
			err := processApplyResponse([]byte(`{"id":null,"metadata":{"id":"gw-202"}}`), true)
			if err != nil {
				t.Fatalf("processApplyResponse() unexpected err: %v", err)
			}
		})

		if !strings.Contains(out, "ID: gw-202") {
			t.Fatalf("stdout = %q, want contains ID: gw-202", out)
		}
	})
}

func withOutputStyle(style string, t *testing.T, fn func()) {
	t.Helper()
	previous := outputStyle
	outputStyle = style
	t.Cleanup(func() {
		outputStyle = previous
	})
	fn()
}

func captureStdoutForIDTest(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}

	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy() failed: %v", err)
	}
	_ = r.Close()
	return buf.String()
}
