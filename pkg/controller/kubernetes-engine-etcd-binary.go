package controller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// etcdReleaseBaseURL は etcd の GitHub Releases のダウンロードベースURL。
	etcdReleaseBaseURL = "https://github.com/etcd-io/etcd/releases/download"

	// DefaultEtcdBinaryCacheDir はダウンロードしたetcdバイナリのキャッシュ先ディレクトリ。
	DefaultEtcdBinaryCacheDir = "/var/lib/marmot/mke/etcd"

	// etcdDownloadTimeout はetcdリリースアセット取得1回あたりのタイムアウト。
	etcdDownloadTimeout = 2 * time.Minute

	// etcdDownloadMaxBytes はダウンロード時に許容する最大サイズ(想定外に大きい
	// レスポンスによるメモリ消費を防ぐための上限)。
	etcdDownloadMaxBytes = 200 * 1024 * 1024
)

var etcdHTTPClient = &http.Client{Timeout: etcdDownloadTimeout}

// etcdDownload は指定URLの内容をバイト列で取得する。テストから差し替え可能にするため
// パッケージ変数として保持する(実ネットワークに依存しないテストを可能にする)。
// ハングや想定外に大きいレスポンスを避けるため、タイムアウトとサイズ上限を設ける。
var etcdDownload = func(rawURL string) ([]byte, error) {
	resp, err := etcdHTTPClient.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, rawURL)
	}
	limited := io.LimitReader(resp.Body, etcdDownloadMaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > etcdDownloadMaxBytes {
		return nil, fmt.Errorf("response for %s exceeds max allowed size (%d bytes)", rawURL, etcdDownloadMaxBytes)
	}
	return data, nil
}

// normalizeEtcdVersionTag は "3.6.8" や "v3.6.8" を受け取り "v3.6.8" 形式の
// リリースタグ名を返す。
func normalizeEtcdVersionTag(version string) (string, error) {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return "", fmt.Errorf("etcd version is empty")
	}
	if !strings.HasPrefix(trimmed, "v") {
		trimmed = "v" + trimmed
	}
	return trimmed, nil
}

// etcdArchFromGOARCH はホストのCPUアーキテクチャからetcdリリースのアーキテクチャ表記を返す。
func etcdArchFromGOARCH(goarch string) (string, error) {
	switch goarch {
	case "amd64", "arm64":
		return goarch, nil
	default:
		return "", fmt.Errorf("unsupported CPU architecture for etcd download: %s", goarch)
	}
}

// EtcdBinaryPath は指定バージョン・キャッシュディレクトリ配下でのetcdバイナリの
// 想定パスを返す(ダウンロード有無に関わらず一意に決まる)。
func EtcdBinaryPath(cacheDir, version string) (string, error) {
	tag, err := normalizeEtcdVersionTag(version)
	if err != nil {
		return "", err
	}
	arch, err := etcdArchFromGOARCH(runtime.GOARCH)
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, tag+"-linux-"+arch, "etcd"), nil
}

// EnsureEtcdBinary は mke.json の etcd_version に対応するetcdバイナリをGitHub Releasesから
// ダウンロードし、cacheDir配下にキャッシュした上でバイナリのパスを返す。CPUアーキテクチャは
// ホスト(runtime.GOARCH)と同じもの、OSはLinux固定とする。ダウンロード後はSHA256SUMSと
// 照合して検証する。既にキャッシュ済みの場合はダウンロードを行わずそのパスを返す(冪等)。
func EnsureEtcdBinary(cacheDir, version string) (string, error) {
	tag, err := normalizeEtcdVersionTag(version)
	if err != nil {
		return "", err
	}
	arch, err := etcdArchFromGOARCH(runtime.GOARCH)
	if err != nil {
		return "", err
	}

	binPath, err := EtcdBinaryPath(cacheDir, version)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(binPath); statErr == nil && !info.IsDir() {
		return binPath, nil
	}

	assetName := fmt.Sprintf("etcd-%s-linux-%s.tar.gz", tag, arch)
	assetURL := fmt.Sprintf("%s/%s/%s", etcdReleaseBaseURL, tag, assetName)
	sumsURL := fmt.Sprintf("%s/%s/SHA256SUMS", etcdReleaseBaseURL, tag)

	archiveBytes, err := etcdDownload(assetURL)
	if err != nil {
		return "", fmt.Errorf("failed to download %s: %w", assetURL, err)
	}
	sumsBytes, err := etcdDownload(sumsURL)
	if err != nil {
		return "", fmt.Errorf("failed to download %s: %w", sumsURL, err)
	}
	if err := verifyEtcdChecksum(archiveBytes, sumsBytes, assetName); err != nil {
		return "", err
	}

	extractedBinary, err := extractEtcdBinaryFromTarGz(archiveBytes)
	if err != nil {
		return "", err
	}

	destDir := filepath.Dir(binPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	// 並行呼び出し時にtmpファイルが衝突しないよう、destDir配下に一意な一時ファイルを作る
	tmpFile, err := os.CreateTemp(destDir, filepath.Base(binPath)+".tmp-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	_, writeErr := tmpFile.Write(extractedBinary)
	closeErr := tmpFile.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return "", writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", closeErr
	}
	// etcdは実行バイナリのため実行権限(0755)を付与する
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := os.Rename(tmpPath, binPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return binPath, nil
}

// verifyEtcdChecksum はSHA256SUMSの内容からassetNameに対応するハッシュ値を探し、
// archiveBytesの実際のSHA256と一致するか検証する。
func verifyEtcdChecksum(archiveBytes, sumsBytes []byte, assetName string) error {
	sum := sha256.Sum256(archiveBytes)
	actual := hex.EncodeToString(sum[:])

	for _, line := range strings.Split(string(sumsBytes), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		hash, name := fields[0], strings.TrimPrefix(fields[1], "*")
		if name != assetName {
			continue
		}
		if !strings.EqualFold(hash, actual) {
			return fmt.Errorf("checksum mismatch for %s: got %s, want %s", assetName, actual, hash)
		}
		return nil
	}
	return fmt.Errorf("checksum entry for %s not found in SHA256SUMS", assetName)
}

// extractEtcdBinaryFromTarGz はtar.gzアーカイブ("etcd-vX.Y.Z-linux-<arch>/etcd"のように
// トップレベルディレクトリを含む構成)の中から "etcd" という名前のファイルを探して返す。
func extractEtcdBinaryFromTarGz(archiveBytes []byte) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = gzr.Close()
	}()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) == "etcd" {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("etcd binary not found in archive")
}
