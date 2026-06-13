package marmotd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForBlockDevice(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when device path exists", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "nbd0p1")
		if err := os.WriteFile(path, []byte("ok"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		if err := waitForBlockDevice(context.Background(), path, 100*time.Millisecond); err != nil {
			t.Fatalf("waitForBlockDevice() error = %v", err)
		}
	})

	t.Run("returns error when timeout expires", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "missing")
		if err := waitForBlockDevice(context.Background(), path, 150*time.Millisecond); err == nil {
			t.Fatalf("expected timeout error")
		}
	})
}
