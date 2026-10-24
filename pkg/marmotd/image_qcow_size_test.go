package marmotd

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// qcowVirtualSizeGB が qemu-img info の virtual-size(バイト)を正しく GB 単位(端数切り上げ)に
// 変換することを確認する(Windows イメージ登録時の Spec.Size 反映に使用)。
func TestQcowVirtualSizeGB(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.qcow2")

	// ちょうど 20GB のイメージを作成する(端数なしのケース)。
	if out, err := exec.Command("qemu-img", "create", "-f", "qcow2", path, "20G").CombinedOutput(); err != nil {
		t.Fatalf("qemu-img create failed: %v, output=%s", err, string(out))
	}

	got, err := qcowVirtualSizeGB(path)
	if err != nil {
		t.Fatalf("qcowVirtualSizeGB() error = %v", err)
	}
	if got != 20 {
		t.Fatalf("qcowVirtualSizeGB() = %d, want %d", got, 20)
	}
}

// GB 境界を超える端数サイズの場合、切り上げられることを確認する。
func TestQcowVirtualSizeGBRoundsUp(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.qcow2")

	// 1GB + 1MB のイメージを作成し、2GB に切り上げられることを確認する。
	if out, err := exec.Command("qemu-img", "create", "-f", "qcow2", path, "1025M").CombinedOutput(); err != nil {
		t.Fatalf("qemu-img create failed: %v, output=%s", err, string(out))
	}

	got, err := qcowVirtualSizeGB(path)
	if err != nil {
		t.Fatalf("qcowVirtualSizeGB() error = %v", err)
	}
	if got != 2 {
		t.Fatalf("qcowVirtualSizeGB() = %d, want %d", got, 2)
	}
}

func TestQcowVirtualSizeGBMissingFile(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available")
	}

	if _, err := qcowVirtualSizeGB("/nonexistent/path/does-not-exist.qcow2"); err == nil {
		t.Fatalf("expected error for missing file")
	}
}
