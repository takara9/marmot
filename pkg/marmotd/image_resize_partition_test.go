package marmotd

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// makeFakeNbdPartitions は t.TempDir() 配下に /sys/block/<base>/<base>pN 相当の
// 疑似ディレクトリを作成する(Rocky 9 のようなGPT複数パーティション構成を模擬する)。
func makeFakeNbdPartitions(t *testing.T, base string, partitionNumbers []int) string {
	t.Helper()
	root := t.TempDir()
	diskDir := filepath.Join(root, base)
	if err := os.MkdirAll(diskDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for _, n := range partitionNumbers {
		partDir := filepath.Join(diskDir, base+"p"+strconv.Itoa(n))
		if err := os.MkdirAll(partDir, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
	}
	return root
}

func TestFindLastPartitionNumberInRoot(t *testing.T) {
	t.Parallel()

	t.Run("single partition (Ubuntu/Alpine相当)", func(t *testing.T) {
		t.Parallel()
		root := makeFakeNbdPartitions(t, "nbd0", []int{1})

		got, err := findLastPartitionNumberInRoot(root, "/dev/nbd0")
		if err != nil {
			t.Fatalf("findLastPartitionNumberInRoot() error = %v", err)
		}
		if got != 1 {
			t.Fatalf("findLastPartitionNumberInRoot() = %d, want 1", got)
		}
	})

	t.Run("multiple partitions (Rocky 9相当)", func(t *testing.T) {
		t.Parallel()
		root := makeFakeNbdPartitions(t, "nbd0", []int{1, 2, 3, 4})

		got, err := findLastPartitionNumberInRoot(root, "/dev/nbd0")
		if err != nil {
			t.Fatalf("findLastPartitionNumberInRoot() error = %v", err)
		}
		if got != 4 {
			t.Fatalf("findLastPartitionNumberInRoot() = %d, want 4", got)
		}
	})

	t.Run("no partitions found", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()

		if _, err := findLastPartitionNumberInRoot(root, "/dev/nbd0"); err == nil {
			t.Fatalf("expected error when no partitions exist")
		}
	})
}

func TestWaitForLastPartitionNumberInRoot(t *testing.T) {
	t.Parallel()

	t.Run("succeeds once partitions appear", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()

		go func() {
			time.Sleep(50 * time.Millisecond)
			diskDir := filepath.Join(root, "nbd0")
			_ = os.MkdirAll(filepath.Join(diskDir, "nbd0p1"), 0755)
			_ = os.MkdirAll(filepath.Join(diskDir, "nbd0p2"), 0755)
		}()

		got, err := waitForLastPartitionNumberInRoot(context.Background(), root, "/dev/nbd0", 2*time.Second)
		if err != nil {
			t.Fatalf("waitForLastPartitionNumberInRoot() error = %v", err)
		}
		if got != 2 {
			t.Fatalf("waitForLastPartitionNumberInRoot() = %d, want 2", got)
		}
	})

	t.Run("returns error on timeout", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()

		if _, err := waitForLastPartitionNumberInRoot(context.Background(), root, "/dev/nbd0", 150*time.Millisecond); err == nil {
			t.Fatalf("expected timeout error")
		}
	})

	t.Run("returns context error when canceled", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, err := waitForLastPartitionNumberInRoot(ctx, root, "/dev/nbd0", time.Second); err == nil {
			t.Fatalf("expected context canceled error")
		}
	})
}

func TestDetectFilesystemTypeInvalidDevice(t *testing.T) {
	t.Parallel()

	// 存在しないデバイスを指定した場合、lsblk がエラーになることを確認する。
	if _, err := detectFilesystemType(context.Background(), "/dev/marmot-test-nonexistent"); err == nil {
		t.Fatalf("expected error for nonexistent device")
	}
}
