package marmotd

// イメージの情報管理の関数群

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

const (
	IMAGE_POOL = "/var/lib/marmot/images"
)

// CreateNewImage は、指定されたIDのイメージを新規作成する関数
// コントローラーから呼び出されることを想定している
func (m *Marmot) CreateNewImage(id string) (*api.Image, error) {
	slog.Debug("Creating image", "imgId", id)

	// /var/lib/marmot/imagesの存在をチェックして、無ければ作成する
	if _, err := os.Stat(IMAGE_POOL); os.IsNotExist(err) {
		err := os.Mkdir(IMAGE_POOL, 0755)
		if err != nil {
			slog.Error("Failed to create image pool directory", "err", err)
			return nil, err
		}
	}

	imageDir := filepath.Join(IMAGE_POOL, id)
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		slog.Error("Failed to create image directory", "imgId", id, "err", err)
		return nil, err
	}

	image, err := m.Db.GetImage(id)
	if err != nil {
		slog.Error("Failed to get image data from DB", "imgId", id, "err", err)
		return nil, err
	}

	if image.Spec == nil || image.Spec.SourceUrl == nil || *image.Spec.SourceUrl == "" {
		slog.Error("sourceUrl is empty", "imgId", id)
		return nil, fmt.Errorf("sourceUrl is empty: id=%s", id)
	}
	src := *image.Spec.SourceUrl

	downloadPath, err := resolveImagePath(imageDir, src)
	if err != nil {
		slog.Error("Failed to resolve image path", "imgId", id, "sourceUrl", src, "err", err)
		return nil, err
	}

	// イメージをダウンロードする
	if err := downloadImage(src, downloadPath); err != nil {
		slog.Error("Failed to download image", "imgId", id, "url", src, "err", err)
		return nil, err
	}

	// イメージがQCOW2であることを確認する
	if err := validateQcowV2Image(downloadPath); err != nil {
		_ = os.Remove(downloadPath)
		slog.Error("Downloaded file is not QEMU QCOW Image (v2)", "imgId", id, "path", downloadPath, "err", err)
		return nil, err
	}

	// QCOW2イメージをカスタマイズする（SSH有効化、ネットワーク設定など）
	if err := customizeQcowImage(downloadPath); err != nil {
		slog.Error("Failed to customize QCOW2 image", "imgId", id, "path", downloadPath, "err", err)
		return nil, err
	}

	// イメージを16GBに拡張する
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	bootVolumeSizeGB := 16
	if err := resizeCustomizedImage(ctx, downloadPath, bootVolumeSizeGB); err != nil {
		return nil, err
	}

	// イメージをLVにコピーする
	lvPath, lvName, err := createBootableLVFromQCOW2(ctx, id, downloadPath)
	if err != nil {
		slog.Error("Failed to create bootable LV from QCOW2", "imgId", id, "path", downloadPath, "err", err)
		return nil, err
	}

	image.Spec.Kind = util.StringPtr("os")
	image.Spec.Type = util.StringPtr("qcow2")

	image.Spec.VolumeGroup = util.StringPtr("vg1")
	image.Spec.LogicalVolume = util.StringPtr(lvName)
	image.Spec.LvPath = util.StringPtr(lvPath)

	image.Spec.Qcow2Path = util.StringPtr(downloadPath)
	image.Spec.Size = util.IntPtrInt(bootVolumeSizeGB)
	image.Status.Status = util.IntPtrInt(db.IMAGE_AVAILABLE)

	// ここで保存されていない！
	if err := m.Db.UpdateImage(id, image); err != nil {
		slog.Error("Failed to update image data in DB", "imgId", id, "err", err)
		return nil, err
	}

	return &image, nil
}

func (m *Marmot) UpdateImageStatus(id string, status int) error {
	slog.Debug("Updating image status", "imgId", id, "status", status)
	image, err := m.Db.GetImage(id)
	if err != nil {
		slog.Error("Failed to get image data from DB", "imgId", id, "err", err)
		return err
	}
	image.Status.Status = util.IntPtrInt(status)
	if err := m.Db.UpdateImage(id, image); err != nil {
		slog.Error("Failed to update image status in DB", "imgId", id, "err", err)
		return err
	}
	return nil
}

func (m *Marmot) GetImages() ([]api.Image, error) {
	slog.Debug("Getting images")
	return m.Db.GetImages()
}

func (m *Marmot) GetImage(id string) (api.Image, error) {
	slog.Debug("Getting image", "imgId", id)
	return m.Db.GetImage(id)
}

func (m *Marmot) DeleteImage(id string) error {
	slog.Debug("Deleting image", "imgId", id)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	image, err := m.Db.GetImage(id)
	if err != nil {
		slog.Error("Failed to get image data from DB for deletion", "imgId", id, "err", err)
		return err
	}

	if image.Spec != nil {
		if image.Spec.LvPath != nil && strings.TrimSpace(*image.Spec.LvPath) != "" {
			lvPath := strings.TrimSpace(*image.Spec.LvPath)
			slog.Debug("*** Attempting to remove logical volume ***", "imgId", id, "lvPath", lvPath)
			if err := runCmd(ctx, "lvdisplay", lvPath); err == nil {
				for i := 0; i < 10; i++ {
					err := runCmd(ctx, "lvremove", "-y", lvPath)
					if err == nil {
						slog.Debug("Logical volume removed successfully", "imgId", id, "lvPath", lvPath)
						break
					}
					slog.Warn("Failed to remove logical volume, retrying...", "imgId", id, "lvPath", lvPath, "attempt", i+1, "err", err)
					time.Sleep(3 * time.Second)
				}
			} else {
				slog.Debug("Logical volume not found, skip remove", "imgId", id, "lvPath", lvPath)
			}
		}

		if image.Spec.Qcow2Path != nil && strings.TrimSpace(*image.Spec.Qcow2Path) != "" {
			qcowPath := strings.TrimSpace(*image.Spec.Qcow2Path)
			if err := os.Remove(qcowPath); err != nil && !os.IsNotExist(err) {
				slog.Error("Failed to remove qcow2 file", "imgId", id, "path", qcowPath, "err", err)
				return err
			}
		}
	}

	imageDir := filepath.Join(IMAGE_POOL, id)
	if err := os.RemoveAll(imageDir); err != nil {
		slog.Error("Failed to remove image directory", "imgId", id, "path", imageDir, "err", err)
		return err
	}

	return m.Db.DeleteImage(id)
}

func (m *Marmot) UpdateImage(id string, image api.Image) error {
	slog.Debug("Updating image", "imgId", id)
	return m.Db.UpdateImage(id, image)
}

func resolveImagePath(imageDir, sourceURL string) (string, error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", fmt.Errorf("invalid sourceUrl: %w", err)
	}
	name := filepath.Base(u.Path)
	if name == "." || name == "/" || strings.TrimSpace(name) == "" {
		name = "image.bin"
	}
	return filepath.Join(imageDir, name), nil
}

func downloadImage(sourceURL, destPath string) error {
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(sourceURL)
	if err != nil {
		return fmt.Errorf("http get failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %s", resp.Status)
	}

	tmpPath := destPath + ".part"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file failed: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("copy response body failed: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sync temp file failed: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file failed: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file failed: %w", err)
	}
	return nil
}

func validateQcowV2Image(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open image file failed: %w", err)
	}
	defer f.Close()

	header := make([]byte, 8)
	if _, err := io.ReadFull(f, header); err != nil {
		return fmt.Errorf("read image header failed: %w", err)
	}

	if string(header[:4]) != "QFI\xfb" {
		return fmt.Errorf("invalid qcow magic: %x", header[:4])
	}

	version := binary.BigEndian.Uint32(header[4:8])
	if version < 2 {
		return fmt.Errorf("unsupported qcow version: %d", version)
	}

	return nil
}

func customizeQcowImage(imagePath string) error {
	netplanConfig := "network:\n" +
		"  version: 2\n" +
		"  ethernets:\n" +
		"    enp1s0:\n" +
		"      dhcp4: false\n" +
		"      dhcp6: false\n" +
		"    enp2s0:\n" +
		"      dhcp4: false\n" +
		"      dhcp6: false\n" +
		"    enp7s0:\n" +
		"      dhcp4: false\n" +
		"      dhcp6: false\n" +
		"    enp8s0:\n" +
		"      dhcp4: false\n" +
		"      dhcp6: false\n"

	args := []string{
		"-a", imagePath,
		"--root-password", "password:ubuntu",
		"--edit", "/etc/ssh/sshd_config: s/^#?PermitRootLogin.*/PermitRootLogin yes/",
		"--edit", "/etc/ssh/sshd_config: s/^#?PasswordAuthentication.*/PasswordAuthentication yes/",
		"--run-command", "rm /etc/ssh/sshd_config.d/60-cloudimg-settings.conf",
		"--run-command", "ssh-keygen -A",
		"--run-command", "systemctl enable ssh",
		"--run-command", "systemctl restart ssh",
		"--write", "/etc/netplan/00-nic.yaml:" + netplanConfig,
	}

	cmd := exec.Command("virt-customize", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("virt-customize failed: %w, output: %s", err, strings.TrimSpace(string(output)))
	}

	slog.Debug("virt-customize completed", "imagePath", imagePath, "output", strings.TrimSpace(string(output)))
	return nil
}

// resizeCustomizedImageTo16GB は、QCOW2イメージを16GBへ拡張し、
// パーティションとファイルシステムを拡張する。
func resizeCustomizedImage(ctx context.Context, imageTemplatePath string, volSizeGB int) error {
	nbdDev := "/dev/nbd1"
	partDev := "/dev/nbd1p1"

	// 失敗時でも切断を試みる
	connected := false
	defer func() {
		if connected {
			_ = runCmd(ctx, "qemu-nbd", "-d", nbdDev)
		}
	}()

	slog.Info("Resizing image and extending partition", "image", imageTemplatePath)

	if err := runCmd(ctx, "modprobe", "nbd", "max_part=8"); err != nil {
		return err
	}
	size := fmt.Sprintf("%dG", volSizeGB)
	if err := runCmd(ctx, "qemu-img", "resize", imageTemplatePath, size); err != nil {
		return err
	}
	if err := runCmd(ctx, "qemu-nbd", "-c", nbdDev, imageTemplatePath); err != nil {
		return err
	}
	connected = true

	time.Sleep(3 * time.Second)

	if err := runCmd(ctx, "parted", nbdDev, "--fix", "--script", "resizepart", "1", "100%"); err != nil {
		return err
	}

	time.Sleep(3 * time.Second)

	if err := runCmd(ctx, "e2fsck", "-f", partDev, "-y"); err != nil {
		return err
	}
	if err := runCmd(ctx, "resize2fs", partDev); err != nil {
		return err
	}

	if err := runCmd(ctx, "qemu-nbd", "-d", nbdDev); err != nil {
		return err
	}
	connected = false

	time.Sleep(3 * time.Second)

	if err := runCmd(ctx, "qemu-img", "info", imageTemplatePath); err != nil {
		return err
	}

	return nil
}

// runCmd はコマンド実行とエラー出力整形を行うヘルパー。
func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w, output=%s",
			name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	slog.Debug("command succeeded", "cmd", name, "args", args, "output", strings.TrimSpace(string(out)))
	return nil
}

// CreateBootableLVFromQCOW2 creates /dev/vg1/<imageID> (16G) and writes qcow2 image into it.
func createBootableLVFromQCOW2(ctx context.Context, imageID, qcow2Path string) (string, string, error) {
	const (
		vgName = "vg1"
		lvSize = "16G"
	)
	bootVolName := "boot-" + imageID

	lvName, err := normalizeLVName(bootVolName)
	if err != nil {
		return "", "", err
	}
	lvPath := fmt.Sprintf("/dev/%s/%s", vgName, lvName)

	// 既存LVがあればエラーにする（上書き事故防止）
	if err := runCmd(ctx, "lvdisplay", lvPath); err == nil {
		return "", "", fmt.Errorf("logical volume already exists: %s", lvPath)
	}

	created := false
	defer func() {
		// 途中失敗時の後始末（必要ならコメントアウト）
		if !created {
			_ = runCmd(ctx, "lvremove", "-y", lvPath)
		}
	}()

	// 1) 16G のLV作成
	if err := runCmd(ctx, "lvcreate", "-L", lvSize, "-n", lvName, "-y", vgName); err != nil {
		return "", "", fmt.Errorf("lvcreate failed: %w", err)
	}

	// 2) QCOW2 -> RAW を LV へ直接書き込み
	//    これでパーティションテーブルやブートローダも複製される
	if err := runCmd(ctx, "qemu-img", "convert", "-q", "-f", "qcow2", "-O", "raw", qcow2Path, lvPath); err != nil {
		_ = runCmd(ctx, "lvremove", "-y", lvPath)
		return "", "", fmt.Errorf("qemu-img convert failed: %w", err)
	}

	// 3) 念のため情報確認（任意）
	if err := runCmd(ctx, "lvdisplay", lvPath); err != nil {
		return "", "", fmt.Errorf("lvdisplay after convert failed: %w", err)
	}

	created = true
	return lvPath, lvName, nil
}

func normalizeLVName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("imageID is empty")
	}
	// LVMで扱いやすい文字に制限（英数字/_/./-）
	re := regexp.MustCompile(`[^a-zA-Z0-9_.-]`)
	n := re.ReplaceAllString(s, "-")
	n = strings.Trim(n, "-.")
	if n == "" {
		return "", fmt.Errorf("invalid imageID after normalization")
	}
	if len(n) > 120 {
		n = n[:120]
	}
	return n, nil
}
