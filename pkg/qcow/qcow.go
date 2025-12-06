package qcow

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
)

// QCOW2ボリュームの存在チェック
func IsExist(path string) error {
	slog.Debug("Checking QCOW2 volume existence", "path", path)
	cmd := exec.Command("qemu-img", "info", path)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("QCOW2 volume does not exist at path: %s", path)
	}
	return nil
}

// QCOW2リュームの作成、サイズはMB単位
func CreateQcow(path string, size int) error {
	slog.Debug("Creating QCOW2 volume", "path", path)
	cmd := exec.Command("qemu-img", "create", "-f", "qcow2", path, fmt.Sprintf("%dM", size))
	//err := cmd.Run()
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("qemu-img create failed", "output", string(output))
		return fmt.Errorf("Failed to create QCOW2 volume at path: %s, error: %v", path, err)
	}
	return nil
}

// QCOW2 ボリュームのコピー
func CopyQcow(srcPath string, destPath string) error {
	slog.Debug("Copying QCOW2 volume", "srcPath", srcPath, "destPath", destPath)
	cmd := exec.Command("qemu-img", "convert", "-O", "qcow2", srcPath, destPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("qemu-img copy failed", "output", string(output))
		return fmt.Errorf("Failed to copy QCOW2 volume from %s to %s, error: %v", srcPath, destPath, err)
	}
	return nil
}

// QCOW2ボリュームの削除
func RemoveQcow(path string) error {
	slog.Debug("Removing QCOW2 volume", "path", path)
	cmd := exec.Command("rm", "-f", path)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Failed to remove QCOW2 volume at path: %s, error: %v", path, err)
	}
	return nil
}

// スナップショットの作成
func CreateSnapshotQcow(path string, snapshotName string) error {
	slog.Debug("Creating snapshot for QCOW2 volume", "path", path)
	cmd := exec.Command("qemu-img", "snapshot", "-c", snapshotName, path)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Failed to create snapshot for QCOW2 volume at path: %s, error: %v", path, err)
	}
	return nil
}

// スナップショットのリスト取得
func ListSnapshotsQcow(path string) ([]string, error) {
	slog.Debug("Listing snapshots for QCOW2 volume", "path", path)
	cmd := exec.Command("qemu-img", "snapshot", "-l", path)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Failed to list snapshots for QCOW2 volume at path: %s, error: %v", path, err)
	}

	slog.Debug("Snapshots listed", "output length", len(output), "output", string(output))
	if len(output) == 0 {
		slog.Debug("No snapshots found for QCOW2 volume", "path", path)
		return nil, fmt.Errorf("No snapshots found for QCOW2 volume at path: %s", path)
	}

	// 出力のパース
	lines := string(output)
	var snapshots []string
	for _, line := range splitLines(lines)[1:] { // ヘッダー行をスキップ
		fields := splitFields(line)
		if len(fields) >= 2 {
			snapshots = append(snapshots, fields[1]) // スナップショット名は2番目のフィールド
		}
	}
	return snapshots, nil
}

// スナップショットからボリュームを復元
func RestoreSnapshotQcow(path string, snapshotName string) error {
	slog.Debug("Restoring snapshot for QCOW2 volume", "path", path)
	cmd := exec.Command("qemu-img", "snapshot", "-a", snapshotName, path)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Failed to restore snapshot for QCOW2 volume at path: %s, error: %v", path, err)
	}
	return nil
}

// スナップショットの削除
func DeleteSnapshotQcow(path string, snapshotName string) error {
	slog.Debug("Deleting snapshot for QCOW2 volume", "path", path)
	cmd := exec.Command("qemu-img", "snapshot", "-d", snapshotName, path)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Failed to delete snapshot for QCOW2 volume at path: %s, error: %v", path, err)
	}
	return nil
}

// ボリュームグループの総量量と空きチェック
func CheckQcow(path string) (uint64, uint64, error) {
	slog.Debug("Checking QCOW2 volume size", "path", path)
	//var total_sz uint64
	//var free_sz uint64

	// qemu-img infoコマンドで情報取得
	cmd := exec.Command("qemu-img", "info", "--output=json", path)
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("Failed to get QCOW2 volume info at path: %s, error: %v", path, err)
	}

	// JSONパース
	type QcowInfo struct {
		Size       uint64 `json:"size"`
		ActualSize uint64 `json:"actual-size"`
	}
	var info QcowInfo
	err = json.Unmarshal(output, &info)
	if err != nil {
		return 0, 0, fmt.Errorf("Failed to parse QCOW2 volume info JSON at path: %s, error: %v", path, err)
	}

	return info.Size, info.Size - info.ActualSize, nil
}

// ヘルパー関数: 文字列を行ごとに分割
func splitLines(s string) []string {
	var lines []string
	currentLine := ""
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, currentLine)
			currentLine = ""
		} else {
			currentLine += string(r)
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	return lines
}

// ヘルパー関数: 文字列をフィールドごとに分割
func splitFields(s string) []string {
	var fields []string
	currentField := ""
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if currentField != "" {
				fields = append(fields, currentField)
				currentField = ""
			}
		} else {
			currentField += string(r)
		}
	}
	if currentField != "" {
		fields = append(fields, currentField)
	}
	return fields
}
