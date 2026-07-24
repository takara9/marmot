package marmotd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/takara9/marmot/api"
)

// GenerateCloudInitISO generates a cloud-init ISO with password and SSH key settings.
// usernames が空の場合はデフォルトユーザーに設定し、指定がある場合は各ユーザーを作成します。
func GenerateCloudInitISO(path, password, sshKey string, usernames []string, ansible *api.ServerAnsible) (string, error) {
	// Create temporary directory for cloud-init files
	tempDir, err := os.MkdirTemp("", "cloud-init-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Generate user-data
	userData, err := buildCloudInitUserData(password, sshKey, usernames, ansible)
	if err != nil {
		return "", err
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

	// Generate network-config: cloud-initのネットワーク設定を無効化（50-cloud-init.yamlを生成させない）
	networkConfig := "network:\n  config: disabled\n"
	networkConfigPath := filepath.Join(tempDir, "network-config")
	if err := os.WriteFile(networkConfigPath, []byte(networkConfig), 0644); err != nil {
		err := fmt.Errorf("failed to write network-config: %v", err)
		slog.Error("Cloud-initの作成中に、ネットワーク設定の書き込みに失敗", "error", err)
		return "", err
	}

	// Generate ISO using genisoimage (assuming it's installed)
	if err := os.MkdirAll(path, 0755); err != nil {
		err := fmt.Errorf("failed to create directory: %v", err)
		slog.Error("Cloud-initの作成中に、ディレクトリの作成に失敗", "error", err)
		return "", err
	}
	isoPath := filepath.Join(path, "cloud-init.iso")
	slog.Debug("Cloud-init ISOの生成を開始", "isoPath", isoPath)
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

func buildCloudInitUserData(password, sshKey string, usernames []string, ansible *api.ServerAnsible) (string, error) {
	users := normalizeUsernames(usernames)
	onBootSection, err := buildCloudInitAnsibleOnBootSection(ansible)
	if err != nil {
		return "", err
	}

	if len(users) == 0 {
		base := fmt.Sprintf(`#cloud-config
password: %s
chpasswd: { expire: False }
ssh_authorized_keys:
%s
`, password, formatSSHKeys(sshKey, "  "))
		return base + onBootSection, nil
	}

	var builder strings.Builder
	builder.WriteString("#cloud-config\n")
	builder.WriteString("users:\n")
	for _, username := range users {
		builder.WriteString(fmt.Sprintf("  - name: %s\n", username))
		builder.WriteString("    shell: /bin/bash\n")
		builder.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n")
		builder.WriteString("    ssh_authorized_keys:\n")
		builder.WriteString(formatSSHKeys(sshKey, "      "))
		builder.WriteString("\n")
	}
	if password != "" {
		builder.WriteString("chpasswd:\n")
		builder.WriteString("  list:\n")
		for _, username := range users {
			builder.WriteString(fmt.Sprintf("    - %s:%s\n", username, password))
		}
		builder.WriteString("  expire: False\n")
	}
	builder.WriteString(onBootSection)
	return builder.String(), nil
}

func buildCloudInitAnsibleOnBootSection(ansible *api.ServerAnsible) (string, error) {
	if ansible == nil || ansible.OnBoot == nil || !*ansible.OnBoot {
		return "", nil
	}

	remote := ""
	if ansible.RemotePlaybook != nil {
		remote = strings.TrimSpace(*ansible.RemotePlaybook)
	}
	pullURL := ""
	pullPlaybookYAML := "site.yml"
	if ansible.Pull != nil {
		if ansible.Pull.Url != nil {
			pullURL = strings.TrimSpace(*ansible.Pull.Url)
		}
		if ansible.Pull.PlaybookYaml != nil {
			if trimmed := strings.TrimSpace(*ansible.Pull.PlaybookYaml); trimmed != "" {
				pullPlaybookYAML = trimmed
			}
		}
	}

	if remote == "" && pullURL == "" {
		return "", fmt.Errorf("spec.ansible.remotePlaybook or spec.ansible.pull.url is required when spec.ansible.onBoot is true")
	}
	if remote != "" && pullURL != "" {
		return "", fmt.Errorf("spec.ansible.remotePlaybook cannot be combined with spec.ansible.pull.url")
	}
	if optionalTrimmedString(ansible.Playbook) != "" || optionalTrimmedString(ansible.Inventory) != "" {
		return "", fmt.Errorf("spec.ansible.onBoot=true cannot be combined with spec.ansible.playbook or spec.ansible.inventory")
	}
	if pullURL != "" {
		section := strings.Join([]string{
			"package_update: true",
			"package_upgrade: true",
			"packages:",
			"  - git",
			"ansible:",
			"  install_method: distro",
			"  package_name: ansible",
			"  pull:",
			"    url: " + strconv.Quote(pullURL),
			"    playbook_name: " + strconv.Quote(pullPlaybookYAML),
			"",
		}, "\n")

		return section, nil
	}

	extraArgs := ""
	if ansible.ExtraArgs != nil {
		quoted := make([]string, 0, len(*ansible.ExtraArgs))
		for _, arg := range *ansible.ExtraArgs {
			trimmed := strings.TrimSpace(arg)
			if trimmed == "" {
				continue
			}
			quoted = append(quoted, shellSingleQuote(trimmed))
		}
		if len(quoted) > 0 {
			extraArgs = " " + strings.Join(quoted, " ")
		}
	}

	remoteQuoted := shellSingleQuote(remote)
	section := fmt.Sprintf(`runcmd:
  - |
    set -eu
    if ! command -v ansible-playbook >/dev/null 2>&1; then
      if command -v apt-get >/dev/null 2>&1; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get update || true
        apt-get install -y ansible curl || apt-get install -y ansible wget
      elif command -v apk >/dev/null 2>&1; then
        apk add --no-cache ansible curl || apk add --no-cache ansible wget
      fi
    fi
    workdir=/var/lib/marmot/ansible-onboot
    mkdir -p "$workdir"
    if command -v curl >/dev/null 2>&1; then
      curl -fsSL %s -o "$workdir/playbook.yml"
    elif command -v wget >/dev/null 2>&1; then
      wget -qO "$workdir/playbook.yml" %s
    else
      echo "curl/wget not found" >&2
      exit 1
    fi
    ansible-playbook -i localhost, -c local "$workdir/playbook.yml"%s
`, remoteQuoted, remoteQuoted, extraArgs)

	return section, nil
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func optionalTrimmedString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func normalizeUsernames(usernames []string) []string {
	normalized := make([]string, 0, len(usernames))
	for _, username := range usernames {
		if trimmed := strings.TrimSpace(username); trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	return normalized
}
