package marmotd

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDownloadImageWithContext_FileScheme は、sourceUrl が file:// スキームの場合に
// downloadImageWithContext がローカルファイルコピーに分岐し、destPath へ正しく
// コピーされることを確認する(Phase2: Windows インストールISOの受け渡し対応)。
func TestDownloadImageWithContext_FileScheme(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.iso")
	content := []byte("dummy iso content for file:// scheme test")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("failed to prepare source file: %v", err)
	}

	destPath := filepath.Join(dir, "dest.iso")
	sourceURL := "file://" + srcPath

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := downloadImageWithContext(ctx, sourceURL, destPath); err != nil {
		t.Fatalf("downloadImageWithContext() error = %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read dest file: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("copied content mismatch: got %q, want %q", got, content)
	}

	// コピー完了後、一時ファイル(.part)が残存していないことを確認する。
	if _, err := os.Stat(destPath + ".part"); !os.IsNotExist(err) {
		t.Fatalf("expected .part file to be removed, stat err = %v", err)
	}
}

// 存在しないローカルファイルを指定した場合はエラーになることを確認する。
func TestDownloadImageWithContext_FileSchemeMissingSource(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "dest.iso")
	sourceURL := "file://" + filepath.Join(dir, "does-not-exist.iso")

	err := downloadImageWithContext(context.Background(), sourceURL, destPath)
	if err == nil {
		t.Fatalf("expected error for missing source file, got nil")
	}
}

// file://host/path のようにホストを伴う指定は、ローカルパス以外を意図している可能性があるため
// 明示的に非対応エラーとすることを確認する。
func TestDownloadImageWithContext_FileSchemeWithHostUnsupported(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "dest.iso")

	err := downloadImageWithContext(context.Background(), "file://remote-host/path/to.iso", destPath)
	if err == nil {
		t.Fatalf("expected error for file scheme with host, got nil")
	}
}

// 送信元がディレクトリの場合はエラーになることを確認する。
func TestCopyLocalImageFileRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "dest.iso")
	u, err := url.Parse("file://" + dir)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	if err := copyLocalImageFile(context.Background(), u, destPath); err == nil {
		t.Fatalf("expected error when source path is a directory")
	}
}

// http(s) スキームの既存動作には影響しないことを確認する(非回帰)。
// 無効なURLでエラーになることのみを確認し、実際のネットワークアクセスは行わない。
func TestDownloadImageWithContext_HTTPSchemeStillRejectsInvalidURL(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "dest.img")

	err := downloadImageWithContext(context.Background(), "http://[::1]:namedport/x", destPath)
	if err == nil {
		t.Fatalf("expected error for malformed http URL, got nil")
	}
}
