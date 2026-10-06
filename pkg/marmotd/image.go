package marmotd

// イメージの情報管理の関数群

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/util"
)

const (
	IMAGE_POOL = "/var/lib/marmot/images"
)

var checkImageVolumeGroup = func(vgName string) error {
	_, _, err := lvm.CheckVG(vgName)
	return err
}

// NBD デバイス利用は排他し、同時実行時の /dev/nbdX 競合を避ける。
var resizeNBDMu sync.Mutex

// CreateNewImage は、指定されたIDのイメージを新規作成する関数 	コントローラーで使用
func (m *Marmot) CreateNewImageManage(id string) (*api.Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), CurrentConfig().ImageCreateFromURLTimeout())
	defer cancel()
	return m.CreateNewImageManageWithContext(ctx, id)
}

// CreateNewImage は、指定されたIDのイメージを新規作成する関数  コントローラーで使用
func (m *Marmot) CreateNewImageManageWithContext(ctx context.Context, id string) (*api.Image, error) {
	slog.Debug("Creating image", "imgId", id)
	if ctx == nil {
		ctx = context.Background()
	}
	operationTimeout := contextTimeoutHint(ctx)

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
	if image.Status == nil {
		image.Status = &api.Status{}
	}
	updateImageMessage := func(message string) error {
		image.Status.Message = util.StringPtr(message)
		if err := m.Db.UpdateImage(id, image); err != nil {
			slog.Error("Failed to update image status in DB", "imgId", id, "err", err)
			return err
		}
		return nil
	}
	markFailed := func(err error) (*api.Image, error) {
		err = wrapDeadlineExceeded(err, "URL からのイメージ作成", operationTimeout)
		return nil, m.markImageCreationFailed(image, err)
	}
	if err := ctx.Err(); err != nil {
		return markFailed(err)
	}

	if err := updateImageMessage("イメージの作成処理を開始"); err != nil {
		return nil, err
	}

	if image.Spec.SourceUrl == nil || *image.Spec.SourceUrl == "" {
		slog.Error("sourceUrl is empty", "imgId", id)
		return markFailed(fmt.Errorf("sourceUrl is empty: id=%s", id))
	}
	src := *image.Spec.SourceUrl

	if err := updateImageMessage("ダウンロード進行中"); err != nil {
		return nil, err
	}

	downloadPath, err := resolveImagePath(imageDir, src)
	if err != nil {
		slog.Error("Failed to resolve image path", "imgId", id, "sourceUrl", src, "err", err)
		return markFailed(err)
	}

	// イメージをダウンロードする
	downloadCtx, downloadCancel := newTimeoutContext(ctx, CurrentConfig().ImageDownloadTimeout())
	defer downloadCancel()
	if err := downloadImageWithRetry(downloadCtx, src, downloadPath); err != nil {
		slog.Error("Failed to download image", "imgId", id, "url", src, "err", err)
		return markFailed(err)
	}

	if err := updateImageMessage("OSイメージを設定中"); err != nil {
		return nil, err
	}

	imageModule, err := resolveImageOSModuleFromImage(image)
	if err != nil {
		slog.Error("resolveImageOSModuleFromImage()", "imgId", id, "err", err)
		return markFailed(err)
	}

	// イメージがQCOW2であることを確認する
	if err := validateQcowV2Image(downloadPath); err != nil {
		_ = os.Remove(downloadPath)
		slog.Error("Downloaded file is not QEMU QCOW Image (v2)", "imgId", id, "path", downloadPath, "err", err)
		return markFailed(err)
	}

	// QCOW2イメージをカスタマイズする（SSH有効化、ネットワーク設定など）
	if err := imageModule.customizeDownloadedImage(ctx, downloadPath); err != nil {
		slog.Error("Failed to customize QCOW2 image", "imgId", id, "path", downloadPath, "err", err)
		return markFailed(err)
	}

	// イメージを16GBに拡張する
	resizeCtx, resizeCancel := newTimeoutContext(ctx, CurrentConfig().ImageResizeTimeout())
	defer resizeCancel()
	bootVolumeSizeGB := 16
	if err := resizeCustomizedImage(resizeCtx, downloadPath, bootVolumeSizeGB); err != nil {
		return markFailed(wrapDeadlineExceeded(err, "QCOW2 イメージ拡張", CurrentConfig().ImageResizeTimeout()))
	}

	image.Spec.Kind = util.StringPtr("os")
	image.Spec.Type = util.StringPtr("qcow2")
	image.Spec.Qcow2Path = util.StringPtr(downloadPath)
	image.Spec.Size = util.IntPtrInt(bootVolumeSizeGB)

	volumeGroup := strings.TrimSpace(CurrentConfig().OSVolumeGroup)
	if err := ensureImageVolumeGroupAvailable(volumeGroup); err != nil {
		slog.Warn("OS volume group unavailable; keep qcow2 image only", "imgId", id, "volumeGroup", volumeGroup, "err", err)
	} else {
		if err := updateImageMessage("OSイメージをロジカルボリュームに転送中"); err != nil {
			return nil, err
		}

		lvPath, lvName, err := createBootableLVFromQCOW2(ctx, id, downloadPath, volumeGroup)
		if err != nil {
			slog.Error("Failed to create bootable LV from QCOW2", "imgId", id, "path", downloadPath, "volumeGroup", volumeGroup, "err", err)
			return markFailed(err)
		}

		image.Spec.VolumeGroup = util.StringPtr(volumeGroup)
		image.Spec.LogicalVolume = util.StringPtr(lvName)
		image.Spec.LvPath = util.StringPtr(lvPath)
	}

	image.Status.StatusCode = db.IMAGE_AVAILABLE
	image.Status.Status = util.StringPtr(db.ImageStatus[db.IMAGE_AVAILABLE])
	image.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	image.Status.Message = nil

	if err := m.Db.UpdateImage(id, image); err != nil {
		slog.Error("Failed to update image data in DB", "imgId", id, "err", err)
		return markFailed(err)
	}
	if err := m.Db.UpdateImageStatus(id, db.IMAGE_AVAILABLE); err != nil {
		slog.Error("UpdateImageStatus()", "imgId", id, "err", err)
		return markFailed(err)
	}

	return &image, nil
}

// イメージ群を取得する関数 （ラップ関数）
func (m *Marmot) GetImagesManage() ([]api.Image, error) {
	slog.Debug("Getting images")
	return m.Db.GetImages()
}

// 指定したIDのイメージを取得する関数 （ラップ関数）
func (m *Marmot) GetImageManage(id string) (api.Image, error) {
	slog.Debug("Getting image", "imgId", id)
	return m.Db.GetImage(id)
}

// ImportImageArchiveWithNode はtgzアーカイブからqcow2を取り出してイメージ登録する。
func (m *Marmot) ImportImageArchiveWithNode(src io.Reader, imageName, nodeName string) (api.Image, error) {
	if src == nil {
		return api.Image{}, fmt.Errorf("archive stream is nil")
	}
	name := strings.TrimSpace(imageName)
	if name == "" {
		return api.Image{}, fmt.Errorf("image name is required")
	}

	if err := os.MkdirAll(IMAGE_POOL, 0755); err != nil {
		return api.Image{}, err
	}

	importDir, err := os.MkdirTemp(IMAGE_POOL, "import-")
	if err != nil {
		return api.Image{}, err
	}
	defer func() {
		_ = os.RemoveAll(importDir)
	}()

	var archiveMeta archiveMetaJSON
	importedQcow2, err := extractFromTGZ(src, importDir, &archiveMeta)
	if err != nil {
		return api.Image{}, err
	}
	if err := validateQcowV2Image(importedQcow2); err != nil {
		return api.Image{}, err
	}

	image, err := m.Db.MakeImportedImageEntry(name, nodeName, importedQcow2)
	if err != nil {
		return api.Image{}, err
	}

	if archiveMeta.OsName != "" {
		image.Spec.OsName = util.StringPtr(archiveMeta.OsName)
	}
	if archiveMeta.OsVersion != "" {
		image.Spec.OsVersion = util.StringPtr(archiveMeta.OsVersion)
	}

	cleanupEntry := func() {
		if err := m.Db.DeleteImage(image.Metadata.Id); err != nil {
			slog.Warn("ImportImageArchiveWithNode() failed to rollback imported image entry", "imageId", image.Metadata.Id, "err", err)
		}
	}

	finalDir := filepath.Join(IMAGE_POOL, image.Metadata.Id)
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		cleanupEntry()
		return api.Image{}, err
	}
	finalQcow2Path := filepath.Join(finalDir, fmt.Sprintf("osimage-%s.qcow2", image.Metadata.Id))
	if err := os.Rename(importedQcow2, finalQcow2Path); err != nil {
		cleanupEntry()
		return api.Image{}, err
	}

	image.Spec.Qcow2Path = util.StringPtr(finalQcow2Path)
	if err := m.Db.UpdateImage(image.Metadata.Id, image); err != nil {
		_ = os.Remove(finalQcow2Path)
		cleanupEntry()
		return api.Image{}, err
	}

	return image, nil
}

// 指定したIDのイメージを削除する関数 （ラップ関数） コントローラーで使用
func (m *Marmot) DeleteImageManage(id string) error {
	slog.Debug("Deleting image", "imgId", id)
	ctx, cancel := context.WithTimeout(context.Background(), CurrentConfig().ImageDeleteTimeout())
	defer cancel()

	image, err := m.Db.GetImage(id)
	if err != nil {
		slog.Error("Failed to get image data from DB for deletion", "imgId", id, "err", err)
		return err
	}

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

	imageDir := filepath.Join(IMAGE_POOL, id)
	if err := os.RemoveAll(imageDir); err != nil {
		slog.Error("Failed to remove image directory", "imgId", id, "path", imageDir, "err", err)
		return err
	}

	return m.Db.DeleteImage(id)
}

// イメージの情報を更新する関数 （ラップ関数） コントローラーで使用
func (m *Marmot) UpdateImageManage(id string, image api.Image) error {
	slog.Debug("Updating image", "imgId", id)
	return m.Db.UpdateImage(id, image)
}

func CheckImageBackingStore(image api.Image) error {
	missing := make([]string, 0, 2)

	if qcow2Path := strings.TrimSpace(util.OrDefault(image.Spec.Qcow2Path, "")); qcow2Path != "" {
		if _, err := os.Stat(qcow2Path); err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, fmt.Sprintf("qcow2 file %s", qcow2Path))
			} else {
				return fmt.Errorf("failed to inspect qcow2 file %s: %w", qcow2Path, err)
			}
		}
	}

	if lvPath := getImageLogicalVolumePath(&image.Spec); lvPath != "" {
		if _, err := os.Stat(lvPath); err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, fmt.Sprintf("logical volume %s", lvPath))
			} else {
				return fmt.Errorf("failed to inspect logical volume %s: %w", lvPath, err)
			}
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing image backing store: %s", strings.Join(missing, ", "))
	}

	return nil
}

func getImageLogicalVolumePath(spec *api.ImageSpec) string {
	if spec == nil {
		return ""
	}

	if lvPath := strings.TrimSpace(util.OrDefault(spec.LvPath, "")); lvPath != "" {
		return lvPath
	}

	volumeGroup := strings.TrimSpace(util.OrDefault(spec.VolumeGroup, ""))
	logicalVolume := strings.TrimSpace(util.OrDefault(spec.LogicalVolume, ""))
	if volumeGroup == "" || logicalVolume == "" {
		return ""
	}

	return filepath.Join("/dev", volumeGroup, logicalVolume)
}

func ensureImageVolumeGroupAvailable(vgName string) error {
	vgName = strings.TrimSpace(vgName)
	if vgName == "" {
		return fmt.Errorf("volume group is empty")
	}
	if err := checkImageVolumeGroup(vgName); err != nil {
		return fmt.Errorf("volume group %s is not available: %w", vgName, err)
	}
	return nil
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
	ctx, cancel := context.WithTimeout(context.Background(), CurrentConfig().ImageDownloadTimeout())
	defer cancel()
	return downloadImageWithContext(ctx, sourceURL, destPath)
}

const (
	imageDownloadRetryAttempts = 3
	imageDownloadRetryDelay    = 5 * time.Second
)

// downloadImageWithRetry は、インストール直後などDNS解決が一時的に不安定な状況を想定し、
// 一時的なネットワークエラー時のみ短い間隔でダウンロードを再試行する。
func downloadImageWithRetry(ctx context.Context, sourceURL, destPath string) error {
	var lastErr error
	for attempt := 1; attempt <= imageDownloadRetryAttempts; attempt++ {
		err := downloadImageWithContext(ctx, sourceURL, destPath)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isTransientDownloadError(err) || attempt == imageDownloadRetryAttempts {
			return err
		}
		slog.Warn("image download transient failure; retrying", "url", sourceURL, "attempt", attempt, "maxAttempts", imageDownloadRetryAttempts, "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(imageDownloadRetryDelay):
		}
	}
	return lastErr
}

// isTransientDownloadError は、DNS解決失敗やコネクション拒否/リセットなど、
// 再試行で回復し得るネットワークエラーかどうかを判定する。
func isTransientDownloadError(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

func downloadImageWithContext(ctx context.Context, sourceURL, destPath string) error {
	timeout := contextTimeoutHint(ctx)
	client := &http.Client{Timeout: CurrentConfig().ImageDownloadTimeout()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return wrapDeadlineExceeded(fmt.Errorf("http get failed: %w", err), "イメージのダウンロード", timeout)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %s", resp.Status)
	}

	tmpPath := destPath + ".part"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file failed: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return wrapDeadlineExceeded(fmt.Errorf("copy response body failed: %w", err), "イメージのダウンロード", timeout)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
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
	defer func() {
		_ = f.Close()
	}()

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
	return customizeQcowImageWithContext(context.Background(), imagePath)
}

func customizeQcowImageWithContext(ctx context.Context, imagePath string) error {
	return customizeUbuntuQcowImageWithContext(ctx, imagePath)
}

func customizeUbuntuQcowImageWithContext(ctx context.Context, imagePath string) error {
	timeout := contextTimeoutHint(ctx)
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
		"--run-command", "rm -f /etc/ssh/sshd_config.d/60-cloudimg-settings.conf",
		"--run-command", "ssh-keygen -A",
		"--run-command", "systemctl enable ssh",
		"--run-command", "systemctl restart ssh",
		"--write", "/etc/netplan/00-nic.yaml:" + netplanConfig,
	}

	cmd := exec.CommandContext(ctx, "virt-customize", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return wrapDeadlineExceeded(fmt.Errorf("virt-customize failed: %w, output: %s", err, strings.TrimSpace(string(output))), "QCOW2 イメージ設定", timeout)
	}

	slog.Debug("virt-customize completed", "imagePath", imagePath, "output", strings.TrimSpace(string(output)))
	return nil
}

func customizeRockyQcowImageWithContext(ctx context.Context, imagePath string) error {
	timeout := contextTimeoutHint(ctx)
	args := []string{
		"-a", imagePath,
		"--root-password", "password:rocky",
		"--edit", "/etc/ssh/sshd_config: s/^#?PermitRootLogin.*/PermitRootLogin yes/",
		"--edit", "/etc/ssh/sshd_config: s/^#?PasswordAuthentication.*/PasswordAuthentication yes/",
		"--run-command", "if ls /etc/ssh/sshd_config.d/*cloud*.conf >/dev/null 2>&1; then rm -f /etc/ssh/sshd_config.d/*cloud*.conf; fi",
		"--run-command", "ssh-keygen -A",
		"--run-command", "systemctl enable sshd",
		"--run-command", "systemctl restart sshd",
	}

	cmd := exec.CommandContext(ctx, "virt-customize", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return wrapDeadlineExceeded(fmt.Errorf("virt-customize failed: %w, output: %s", err, strings.TrimSpace(string(output))), "QCOW2 イメージ設定", timeout)
	}

	slog.Debug("virt-customize completed for rocky", "imagePath", imagePath, "output", strings.TrimSpace(string(output)))
	return nil
}

func customizeAlpineQcowImageWithContext(ctx context.Context, imagePath string) error {
	timeout := contextTimeoutHint(ctx)
	args := []string{
		"-a", imagePath,
		"--root-password", "password:alpine",
		"--edit", "/etc/ssh/sshd_config: s/^#?PermitRootLogin.*/PermitRootLogin yes/",
		"--edit", "/etc/ssh/sshd_config: s/^#?PasswordAuthentication.*/PasswordAuthentication yes/",
		"--run-command", "if [ -f /etc/ssh/sshd_config.d/60-cloudimg-settings.conf ]; then rm -f /etc/ssh/sshd_config.d/60-cloudimg-settings.conf; fi",
		"--run-command", "if ! id -u alpine >/dev/null 2>&1; then adduser -D alpine; fi",
		"--run-command", "echo 'alpine:alpine' | chpasswd",
		"--run-command", "if command -v ssh-keygen >/dev/null 2>&1; then ssh-keygen -A; fi",
		"--run-command", "if command -v rc-update >/dev/null 2>&1; then rc-update add sshd default || true; fi",
	}

	cmd := exec.CommandContext(ctx, "virt-customize", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return wrapDeadlineExceeded(fmt.Errorf("virt-customize failed: %w, output: %s", err, strings.TrimSpace(string(output))), "QCOW2 イメージ設定", timeout)
	}

	slog.Debug("virt-customize completed for alpine", "imagePath", imagePath, "output", strings.TrimSpace(string(output)))
	return nil
}

// resizeCustomizedImageTo16GB は、QCOW2イメージを16GBへ拡張し、
// パーティションとファイルシステムを拡張する。
func resizeCustomizedImage(ctx context.Context, imageTemplatePath string, volSizeGB int) error {
	resizeNBDMu.Lock()
	defer resizeNBDMu.Unlock()

	var nbdDev string
	var partDev string

	// 失敗時でも切断を試みる
	connected := false
	defer func() {
		if connected {
			_ = runCmd(ctx, "qemu-nbd", "-d", nbdDev)
		}
	}()

	slog.Debug("Resizing image and extending partition", "image", imageTemplatePath)

	if err := runCmd(ctx, "modprobe", "nbd", "max_part=8"); err != nil {
		return err
	}
	size := fmt.Sprintf("%dG", volSizeGB)
	if err := runCmd(ctx, "qemu-img", "resize", imageTemplatePath, size); err != nil {
		return err
	}

	var attachErrs []string
	for i := 0; i < 16; i++ {
		candidate, err := findFreeNbdDeviceByIndex(i)
		if err != nil {
			continue
		}
		nbdDev = candidate

		// 念のため stale 接続を切る（未接続なら失敗しても無視）
		_ = runCmd(ctx, "qemu-nbd", "-d", nbdDev)
		// NBDデバイス番号は使い回されるため、前回この番号に接続されていたイメージの
		// パーティション情報がカーネル側に残っている場合がある(CI環境などudevdが
		// 動作していない場合は、qemu-nbd切断後も自動的に消えない)。新しいイメージの
		// パーティション数を誤認しないよう、接続前に明示的に消去しておく
		// (issue #622, 失敗例: 1パーティションのUbuntuイメージに対し resizepart 16 が実行される)。
		_ = runCmd(ctx, "partx", "-d", nbdDev)
		if err := runCmd(ctx, "qemu-nbd", "-c", nbdDev, imageTemplatePath); err != nil {
			attachErrs = append(attachErrs, fmt.Sprintf("%s: %v", nbdDev, err))
			continue
		}
		connected = true
		break
	}
	if !connected {
		if len(attachErrs) == 0 {
			return fmt.Errorf("qemu-nbd attach failed: no free nbd device found")
		}
		return fmt.Errorf("qemu-nbd attach failed: %s", strings.Join(attachErrs, " | "))
	}

	if err := refreshPartitionDevices(ctx, nbdDev); err != nil {
		return err
	}

	partitionTableType, err := detectPartitionTableType(ctx, nbdDev)
	if err != nil {
		slog.Warn("Failed to detect partition table type; treat as unknown", "nbdDevice", nbdDev, "err", err)
	}
	hasPartitionTable := hasPartitionTableType(partitionTableType)
	if hasPartitionTable {
		slog.Debug("Detected partition table on NBD device", "nbdDevice", nbdDev, "ptType", partitionTableType)
	}

	resizeTarget := nbdDev
	usingPartitionTarget := false
	// リサイズ対象は「ディスク上で物理的に最後に位置するパーティション」とする。GPTの
	// パーティション番号は物理的な並び順と一致するとは限らない(例: Ubuntu の cloud image は
	// ルートパーティションの番号が1、bios_grub/ESP/bootが14/15/16だが、物理的にはルートが
	// 最後に配置されている)ため、番号ではなく終了オフセットで判定する必要がある(issue #622)。
	if partNum, err := waitForLastPhysicalPartitionNumber(ctx, nbdDev, 5*time.Second); err == nil {
		partDev = fmt.Sprintf("%sp%d", nbdDev, partNum)
		if err := runCmd(ctx, "parted", nbdDev, "--fix", "--script", "resizepart", strconv.Itoa(partNum), "100%"); err != nil {
			return err
		}

		if err := refreshPartitionDevices(ctx, nbdDev); err != nil {
			return err
		}
		if err := waitForBlockDevice(ctx, partDev, 20*time.Second); err != nil {
			return err
		}
		resizeTarget = partDev
		usingPartitionTarget = true
	} else {
		if hasPartitionTable {
			slog.Warn("Partition table detected; extend wait for partition device instead of falling back to whole disk", "nbdDevice", nbdDev, "ptType", partitionTableType, "err", err)
			if err := refreshPartitionDevices(ctx, nbdDev); err != nil {
				return err
			}
			partNum, err := waitForLastPhysicalPartitionNumber(ctx, nbdDev, 30*time.Second)
			if err != nil {
				return err
			}
			partDev = fmt.Sprintf("%sp%d", nbdDev, partNum)
			resizeTarget = partDev
			usingPartitionTarget = true
		} else {
			slog.Warn("Partition table was not detected; fallback to whole-disk filesystem resize", "nbdDevice", nbdDev, "err", err)
		}
	}

	if usingPartitionTarget {
		if _, err := os.Stat(partDev); err != nil {
			if hasPartitionTable {
				slog.Warn("Partition device disappeared before filesystem resize; retry partition detection", "nbdDevice", nbdDev, "partition", partDev, "ptType", partitionTableType, "err", err)
				if err := refreshPartitionDevices(ctx, nbdDev); err != nil {
					return err
				}
				if err := waitForBlockDevice(ctx, partDev, 30*time.Second); err != nil {
					return err
				}
				resizeTarget = partDev
			} else {
				slog.Warn("Partition device disappeared before filesystem resize; fallback to whole disk", "nbdDevice", nbdDev, "partition", partDev, "err", err)
				resizeTarget = nbdDev
				usingPartitionTarget = false
			}
		}
	}

	if err := growFilesystem(ctx, resizeTarget, nbdDev, partDev, usingPartitionTarget, hasPartitionTable, partitionTableType); err != nil {
		return err
	}

	if err := runCmd(ctx, "qemu-nbd", "-d", nbdDev); err != nil {
		return err
	}
	connected = false

	if err := runQemuImgInfoWithRetry(ctx, imageTemplatePath, 10, 300*time.Millisecond); err != nil {
		return err
	}

	return nil
}

// growFilesystem は resizeTarget 上のファイルシステムを、リサイズ済みのパーティション/ディスク
// サイズまで拡張する。ファイルシステム種別により手順が異なる(ext系はオフラインでresize2fs可能だが、
// xfsはオフラインリサイズに対応していないためマウントしてxfs_growfsする必要がある)。
// Rocky 9 の GenericCloud イメージは /boot・/ ともに xfs のため、xfs 分岐が必要になる(issue #622)。
func growFilesystem(ctx context.Context, resizeTarget, nbdDev, partDev string, usingPartitionTarget, hasPartitionTable bool, partitionTableType string) error {
	fsType, err := detectFilesystemType(ctx, resizeTarget)
	if err != nil {
		slog.Warn("Failed to detect filesystem type; falling back to ext* resize path", "device", resizeTarget, "err", err)
	}

	switch fsType {
	case "xfs":
		return growXFSFilesystem(ctx, resizeTarget)
	case "ext2", "ext3", "ext4", "":
		return growExtFilesystem(ctx, resizeTarget, nbdDev, partDev, usingPartitionTarget, hasPartitionTable, partitionTableType)
	default:
		return fmt.Errorf("unsupported filesystem type for resize: %q (device=%s)", fsType, resizeTarget)
	}
}

// growExtFilesystem は ext2/ext3/ext4 ファイルシステムを resize2fs で拡張する(Ubuntu/Alpineの既定経路)。
func growExtFilesystem(ctx context.Context, resizeTarget, nbdDev, partDev string, usingPartitionTarget, hasPartitionTable bool, partitionTableType string) error {
	if err := runCmd(ctx, "e2fsck", "-f", resizeTarget, "-y"); err != nil {
		if usingPartitionTarget && isMissingBlockDeviceError(err) {
			if hasPartitionTable {
				slog.Warn("Partition device became unavailable during e2fsck; retry on partition", "nbdDevice", nbdDev, "partition", partDev, "ptType", partitionTableType, "err", err)
				if refreshErr := refreshPartitionDevices(ctx, nbdDev); refreshErr != nil {
					return refreshErr
				}
				if waitErr := waitForBlockDevice(ctx, partDev, 30*time.Second); waitErr != nil {
					return waitErr
				}
				resizeTarget = partDev
				if retryErr := runCmd(ctx, "e2fsck", "-f", resizeTarget, "-y"); retryErr != nil {
					return retryErr
				}
			} else {
				slog.Warn("Partition device became unavailable during e2fsck; retry on whole disk", "nbdDevice", nbdDev, "partition", partDev, "err", err)
				resizeTarget = nbdDev
				if retryErr := runCmd(ctx, "e2fsck", "-f", resizeTarget, "-y"); retryErr != nil {
					return retryErr
				}
			}
		} else {
			return err
		}
	}
	return runCmd(ctx, "resize2fs", resizeTarget)
}

// growXFSFilesystem は xfs ファイルシステムを拡張する。xfs_growfs はマウント済みのファイルシステム
// に対してのみ動作する(ext系のresize2fsと異なりオフラインリサイズ不可)ため、一時ディレクトリに
// マウントしてから実行する。
func growXFSFilesystem(ctx context.Context, resizeTarget string) error {
	mountPoint, err := os.MkdirTemp("", "marmot-xfs-grow-")
	if err != nil {
		return fmt.Errorf("create temp mount point for xfs_growfs failed: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(mountPoint)
	}()

	if err := runCmd(ctx, "mount", resizeTarget, mountPoint); err != nil {
		return err
	}
	defer func() {
		if err := runCmd(ctx, "umount", mountPoint); err != nil {
			slog.Error("umount failed after xfs_growfs", "mountPoint", mountPoint, "device", resizeTarget, "err", err)
		}
	}()

	return runCmd(ctx, "xfs_growfs", mountPoint)
}

// detectFilesystemType は devicePath 上のファイルシステム種別を取得する。
func detectFilesystemType(ctx context.Context, devicePath string) (string, error) {
	cmd := exec.CommandContext(ctx, "lsblk", "-n", "-o", "FSTYPE", devicePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("lsblk -n -o FSTYPE %s failed: %w, output=%s", devicePath, err, strings.TrimSpace(string(out)))
	}
	return strings.ToLower(strings.TrimSpace(string(out))), nil
}

func findFreeNbdDeviceByIndex(i int) (string, error) {
	devicePath := fmt.Sprintf("/dev/nbd%d", i)
	sysPath := fmt.Sprintf("/sys/class/block/nbd%d/pid", i)

	if _, err := os.Stat(devicePath); os.IsNotExist(err) {
		return "", fmt.Errorf("device %s does not exist", devicePath)
	}
	if _, err := os.Stat(sysPath); os.IsNotExist(err) {
		return devicePath, nil
	}
	return "", fmt.Errorf("device %s is busy", devicePath)
}

// findLastPhysicalPartitionNumber は nbdDev (例: /dev/nbd0) に現在接続されているイメージの
// パーティションテーブルを parted で直接読み取り、ディスク上で物理的に最後に位置する
// (終了オフセットが最大の)パーティション番号を求める。qemu-img resize で追加された空き領域は
// 物理的にこのパーティションの直後に隣接するため、resizepart の対象として安全に100%まで
// 拡張できるのはこのパーティションだけである。
//
// 当初は「最大のパーティション番号」を対象にしていたが、これは誤りだった。GPTの
// パーティション番号は物理的な並び順とは無関係に採番される。例えば Ubuntu の
// cloud image はルートパーティションを番号「1」とし、bios_grub/ESP/boot には
// 14/15/16 という番号より大きい番号を割り当てている(ただし物理的な配置としては
// ルートパーティションが最後に置かれている)。そのため「最大番号」を基準にすると、
// Ubuntuでは /boot (番号16、物理的には先頭寄り)を誤ってリサイズ対象に選んでしまい、
// 既存パーティションと重なって resizepart が失敗していた(issue #622, #737)。
//
// sysfs のパーティションデバイスノード(/sys/block/<dev>/<dev>pN)やカーネルの
// パーティションスキャン状態には依存しない。NBDデバイス番号は使い回されるため、
// udevd が無い/弱いCI環境では前回接続されていた別イメージのパーティション情報が
// カーネル側に残留することがあり、sysfs を参照する方式では誤ったパーティション番号を
// 拾ってしまう場合がある。parted でオンディスクのパーティションテーブルを直接読むことで、
// この種の残留状態の影響を受けないようにしている。
func findLastPhysicalPartitionNumber(ctx context.Context, nbdDev string) (int, error) {
	cmd := exec.CommandContext(ctx, "parted", "-m", "-s", nbdDev, "unit", "s", "print")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("parted -m -s %s unit s print failed: %w, output=%s", nbdDev, err, strings.TrimSpace(string(out)))
	}
	return parseLastPhysicalPartitionNumberFromPartedOutput(string(out))
}

// parseLastPhysicalPartitionNumberFromPartedOutput は `parted -m -s <dev> unit s print` の
// 出力から、終了オフセット(END)が最大のパーティション番号を求める。ヘッダ行("BYT;")、
// ディスク概要行、qemu-img resize 直後に表示されるGPT不整合の警告メッセージなどは、
// いずれも先頭フィールドが数値にならないため自然に無視される。
func parseLastPhysicalPartitionNumberFromPartedOutput(output string) (int, error) {
	bestNum := 0
	bestEnd := int64(-1)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 3 {
			continue
		}
		num, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		end, err := strconv.ParseInt(strings.TrimSuffix(fields[2], "s"), 10, 64)
		if err != nil {
			continue
		}
		if end > bestEnd {
			bestEnd = end
			bestNum = num
		}
	}
	if bestNum == 0 {
		return 0, fmt.Errorf("no partitions found in parted output")
	}
	return bestNum, nil
}

// waitForLastPhysicalPartitionNumber は findLastPhysicalPartitionNumber が成功するまで
// ポーリングする。qemu-nbd 接続直後は一時的にI/Oが安定しないことがあるため、短時間の
// リトライを行う。
func waitForLastPhysicalPartitionNumber(ctx context.Context, nbdDev string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	for {
		if n, err := findLastPhysicalPartitionNumber(ctx, nbdDev); err == nil {
			return n, nil
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("no partition devices found for %s within %s", nbdDev, timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func waitForBlockDevice(ctx context.Context, devicePath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(devicePath); err == nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("device %s did not appear within %s", devicePath, timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func refreshPartitionDevices(ctx context.Context, nbdDev string) error {
	// qemu-nbd の attach直後はカーネルが非同期にパーティションスキャン中で、
	// partprobe が一過性の busy で exit status 1 になることがあるため短くリトライする。
	if err := retryTransientOp(ctx, 5, 200*time.Millisecond, func() error {
		return runCmd(ctx, "partprobe", nbdDev)
	}, func(attempt int, err error) {
		slog.Warn("partprobe transient failure; retrying", "nbdDevice", nbdDev, "attempt", attempt, "err", err)
	}); err != nil {
		return err
	}
	if err := runCmd(ctx, "partx", "-u", nbdDev); err != nil {
		slog.Warn("partx refresh failed", "nbdDevice", nbdDev, "err", err)
	}
	if err := runCmd(ctx, "udevadm", "settle"); err != nil {
		slog.Warn("udevadm settle failed", "nbdDevice", nbdDev, "err", err)
	}
	return nil
}

// retryTransientOp は op を最大 attempts 回まで実行し、失敗のたびに onRetry で通知して delay 待機する。
func retryTransientOp(ctx context.Context, attempts int, delay time.Duration, op func() error, onRetry func(attempt int, err error)) error {
	if attempts < 1 {
		attempts = 1
	}
	if delay < 0 {
		delay = 0
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		err := op()
		if err == nil {
			return nil
		}
		lastErr = err
		if i == attempts-1 {
			return err
		}
		if onRetry != nil {
			onRetry(i+1, err)
		}
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return lastErr
}

func detectPartitionTableType(ctx context.Context, devicePath string) (string, error) {
	cmd := exec.CommandContext(ctx, "lsblk", "-n", "-o", "PTTYPE", devicePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("lsblk -n -o PTTYPE %s failed: %w, output=%s", devicePath, err, strings.TrimSpace(string(out)))
	}
	return strings.ToLower(strings.TrimSpace(string(out))), nil
}

func hasPartitionTableType(partitionTableType string) bool {
	return strings.TrimSpace(partitionTableType) != ""
}

func runQemuImgInfoWithRetry(ctx context.Context, imagePath string, attempts int, delay time.Duration) error {
	if attempts < 1 {
		attempts = 1
	}
	if delay < 0 {
		delay = 0
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		err := runCmd(ctx, "qemu-img", "info", imagePath)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isQemuImgWriteLockError(err) || i == attempts-1 {
			return err
		}

		slog.Warn("qemu-img info lock contention; retrying", "imagePath", imagePath, "attempt", i+1, "maxAttempts", attempts, "err", err)
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("qemu-img info failed without detailed error")
}

func isMissingBlockDeviceError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "possibly non-existent device") ||
		strings.Contains(msg, "does not exist")
}

func isQemuImgWriteLockError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "failed to get shared \"write\" lock") ||
		(strings.Contains(msg, "is another process using the image") && strings.Contains(msg, "qemu-img info"))
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

// CreateBootableLVFromQCOW2 creates /dev/<volume-group>/<imageID> (16G) and writes qcow2 image into it.
func createBootableLVFromQCOW2(ctx context.Context, imageID, qcow2Path, vgName string) (string, string, error) {
	const (
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
