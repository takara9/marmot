package marmotd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GenerateCloudInitISO generates a cloud-init ISO with password and SSH key settings.
// username が空の場合はデフォルトユーザーに設定し、指定がある場合は新規ユーザーを作成します。
func GenerateCloudInitISO(path, password, sshKey, username string) (string, error) {
	// Create temporary directory for cloud-init files
	tempDir, err := os.MkdirTemp("", "cloud-init-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate user-data
	var userData string
	if username != "" {
		userData = fmt.Sprintf(`#cloud-config
users:
  - name: %s
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
%s
chpasswd:
  list:
    - %s:%s
  expire: False
`, username, formatSSHKeys(sshKey, "      "), username, password)
	} else {
		userData = fmt.Sprintf(`#cloud-config
password: %s
chpasswd: { expire: False }
ssh_authorized_keys:
%s
`, password, formatSSHKeys(sshKey, "  "))
	}

	userDataPath := filepath.Join(tempDir, "user-data")
	if err := os.WriteFile(userDataPath, []byte(userData), 0644); err != nil {
		err := fmt.Errorf("failed to write user-data: %v", err)
		slog.Error("Cloud-initの作成中に、ユーザーデータの書き込みに失敗", "error", err)
		return "", err
	}

	// Generate meta-data (minimal)
	metaData := "#cloud-config\ninstance-id: iid-local01\n"
	metaDataPath := filepath.Join(tempDir, "meta-data")
	if err := os.WriteFile(metaDataPath, []byte(metaData), 0644); err != nil {
		err := fmt.Errorf("failed to write meta-data: %v", err)
		slog.Error("Cloud-initの作成中に、メタデータの書き込みに失敗", "error", err)
		return "", err
	}

	// Generate ISO using genisoimage (assuming it's installed)
	if err := os.MkdirAll(path, 0755); err != nil {
		err := fmt.Errorf("failed to create directory: %v", err)
		slog.Error("Cloud-initの作成中に、ディレクトリの作成に失敗", "error", err)
		return "", err
	}
	isoPath := filepath.Join(path, "cloud-init.iso")
	slog.Info("Cloud-init ISOの生成を開始", "isoPath", isoPath)
	cmd := exec.Command("genisoimage", "-output", isoPath, "-volid", "cidata", "-joliet", "-rock", tempDir)
	if err := cmd.Run(); err != nil {
		err := fmt.Errorf("failed to generate ISO: %v", err)
		slog.Error("Cloud-initの作成中に、ISOの生成に失敗", "error", err)
		return "", err
	}

	return isoPath, nil
}

// formatSSHKeys は改行区切りの公開鍵文字列を cloud-init YAML のリスト形式に変換します。
func formatSSHKeys(sshKey, indent string) string {
	var result []string
	for _, line := range strings.Split(sshKey, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, indent+"- "+line)
		}
	}
	return strings.Join(result, "\n")
}
