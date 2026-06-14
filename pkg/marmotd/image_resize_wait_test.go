package marmotd

import (
	"context"
	"errors"
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

func TestIsMissingBlockDeviceError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "e2fsck missing partition",
			err:  errors.New("e2fsck -f /dev/nbd0p1 -y failed: exit status 8, output=e2fsck: No such file or directory while trying to open /dev/nbd0p1"),
			want: true,
		},
		{
			name: "non existent device message",
			err:  errors.New("Possibly non-existent device?"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("permission denied"),
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isMissingBlockDeviceError(tt.err)
			if got != tt.want {
				t.Fatalf("isMissingBlockDeviceError() = %v, want %v", got, tt.want)
			}
		})
	}
}
