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
	"sync"
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
	if err := downloadImageWithContext(downloadCtx, src, downloadPath); err != nil {
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
	m.Db.UpdateImageStatus(id, db.IMAGE_AVAILABLE)

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

	importedQcow2, err := extractSingleQcow2FromTGZ(src, importDir)
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
		return wrapDeadlineExceeded(fmt.Errorf("copy response body failed: %w", err), "イメージのダウンロード", timeout)
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
		partDev = nbdDev + "p1"

		// 念のため stale 接続を切る（未接続なら失敗しても無視）
		_ = runCmd(ctx, "qemu-nbd", "-d", nbdDev)
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

	_ = runCmd(ctx, "partprobe", nbdDev)
	resizeTarget := nbdDev
	if err := waitForBlockDevice(ctx, partDev, 5*time.Second); err == nil {
		if err := runCmd(ctx, "parted", nbdDev, "--fix", "--script", "resizepart", "1", "100%"); err != nil {
			return err
		}

		_ = runCmd(ctx, "partprobe", nbdDev)
		if err := waitForBlockDevice(ctx, partDev, 20*time.Second); err != nil {
			return err
		}
		resizeTarget = partDev
	} else {
		slog.Warn("Partition device was not detected; fallback to whole-disk filesystem resize", "nbdDevice", nbdDev, "partition", partDev, "err", err)
	}

	if err := runCmd(ctx, "e2fsck", "-f", resizeTarget, "-y"); err != nil {
		return err
	}
	if err := runCmd(ctx, "resize2fs", resizeTarget); err != nil {
		return err
	}

	if err := runCmd(ctx, "qemu-nbd", "-d", nbdDev); err != nil {
		return err
	}
	connected = false

	if err := runCmd(ctx, "qemu-img", "info", imageTemplatePath); err != nil {
		return err
	}

	return nil
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
