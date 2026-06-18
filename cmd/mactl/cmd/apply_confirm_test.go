package cmd

import (
	"os"
	"testing"
)

func TestConfirmDowntimeForServerApplyYes(t *testing.T) {
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}
	defer r.Close()

	os.Stdin = r
	if _, err := w.WriteString("Y\n"); err != nil {
		t.Fatalf("WriteString() failed: %v", err)
	}
	_ = w.Close()

	ok, err := confirmDowntimeForServerApply("server-1")
	if err != nil {
		t.Fatalf("confirmDowntimeForServerApply() unexpected err: %v", err)
	}
	if !ok {
		t.Fatalf("confirmDowntimeForServerApply() = false, want true")
	}
}

func TestConfirmDowntimeForServerApplyNo(t *testing.T) {
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}
	defer r.Close()

	os.Stdin = r
	if _, err := w.WriteString("n\n"); err != nil {
		t.Fatalf("WriteString() failed: %v", err)
	}
	_ = w.Close()

	ok, err := confirmDowntimeForServerApply("server-1")
	if err != nil {
		t.Fatalf("confirmDowntimeForServerApply() unexpected err: %v", err)
	}
	if ok {
		t.Fatalf("confirmDowntimeForServerApply() = true, want false")
	}
}
