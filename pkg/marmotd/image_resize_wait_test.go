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

func TestHasPartitionTableType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty", value: "", want: false},
		{name: "spaces", value: "   ", want: false},
		{name: "gpt", value: "gpt", want: true},
		{name: "dos", value: "dos", want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hasPartitionTableType(tt.value)
			if got != tt.want {
				t.Fatalf("hasPartitionTableType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsQemuImgWriteLockError(t *testing.T) {
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
			name: "write lock error",
			err:  errors.New("qemu-img info /tmp/test.qcow2 failed: exit status 1, output=qemu-img: Failed to get shared \"write\" lock"),
			want: true,
		},
		{
			name: "is another process using image",
			err:  errors.New("qemu-img info /tmp/test.qcow2 failed: Is another process using the image [/tmp/test.qcow2]?"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("qemu-img: Could not open file: no such file or directory"),
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isQemuImgWriteLockError(tt.err)
			if got != tt.want {
				t.Fatalf("isQemuImgWriteLockError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryTransientOp(t *testing.T) {
	t.Parallel()

	t.Run("succeeds after transient failures", func(t *testing.T) {
		t.Parallel()

		calls := 0
		var retries []int
		err := retryTransientOp(context.Background(), 5, time.Millisecond, func() error {
			calls++
			if calls < 3 {
				return errors.New("partprobe /dev/nbd0 failed: exit status 1, output=")
			}
			return nil
		}, func(attempt int, _ error) {
			retries = append(retries, attempt)
		})
		if err != nil {
			t.Fatalf("retryTransientOp() error = %v, want nil", err)
		}
		if calls != 3 {
			t.Fatalf("calls = %d, want 3", calls)
		}
		if len(retries) != 2 {
			t.Fatalf("retries = %v, want 2 entries", retries)
		}
	})

	t.Run("returns last error when all attempts fail", func(t *testing.T) {
		t.Parallel()

		calls := 0
		wantErr := errors.New("partprobe /dev/nbd0 failed: exit status 1, output=")
		err := retryTransientOp(context.Background(), 3, time.Millisecond, func() error {
			calls++
			return wantErr
		}, nil)
		if !errors.Is(err, wantErr) {
			t.Fatalf("retryTransientOp() error = %v, want %v", err, wantErr)
		}
		if calls != 3 {
			t.Fatalf("calls = %d, want 3", calls)
		}
	})

	t.Run("stops retrying when context is done", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		calls := 0
		err := retryTransientOp(ctx, 3, time.Millisecond, func() error {
			calls++
			return errors.New("busy")
		}, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("retryTransientOp() error = %v, want context.Canceled", err)
		}
		if calls != 1 {
			t.Fatalf("calls = %d, want 1", calls)
		}
	})
}
