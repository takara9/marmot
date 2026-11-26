package marmotd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveServerCloudInitISORemovesISOAndEmptyDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "srv-1")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cloud-init.iso"), []byte("iso"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := removeServerCloudInitISO(base, "srv-1"); err != nil {
		t.Fatalf("removeServerCloudInitISO() unexpected err: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("directory must be removed, stat err = %v", err)
	}
}

func TestRemoveServerCloudInitISOIsIdempotent(t *testing.T) {
	base := t.TempDir()
	if err := removeServerCloudInitISO(base, "missing"); err != nil {
		t.Fatalf("removeServerCloudInitISO() on missing dir unexpected err: %v", err)
	}
}

func TestRemoveServerCloudInitISOKeepsDirWithOtherFiles(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "srv-2")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cloud-init.iso"), []byte("iso"), 0644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := removeServerCloudInitISO(base, "srv-2"); err == nil {
		t.Fatalf("expected error when directory is not empty")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unrelated file must be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cloud-init.iso")); !os.IsNotExist(err) {
		t.Fatalf("iso must be removed, stat err = %v", err)
	}

	// 再試行: 余計なファイルが無くなれば成功する
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	if err := removeServerCloudInitISO(base, "srv-2"); err != nil {
		t.Fatalf("retry unexpected err: %v", err)
	}
}

func TestRemoveServerCloudInitISORejectsUnsafeID(t *testing.T) {
	base := t.TempDir()
	for _, id := range []string{"", " ", ".", "..", "../x", "a/b"} {
		if err := removeServerCloudInitISO(base, id); err == nil {
			t.Fatalf("expected error for id %q", id)
		}
	}
}
