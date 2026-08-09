package controller

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const ansiblePlaybookLogDir = "/var/lib/marmot/ansible-playbooks/logs"

var ansiblePlaybookLogDirPath = ansiblePlaybookLogDir

func ansiblePlaybookLogPath(resourceName, resourceID string) (string, error) {
	name := sanitizeLogToken(strings.TrimSpace(resourceName), "resource")
	id := sanitizeLogToken(strings.TrimSpace(resourceID), "unknown")

	if err := os.MkdirAll(ansiblePlaybookLogDirPath, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(ansiblePlaybookLogDirPath, fmt.Sprintf("%s-%s.log", name, id)), nil
}

func resourceIDFromPlaybookPath(playbookPath, filePrefix string) string {
	trimmedPath := strings.TrimSpace(playbookPath)
	if trimmedPath == "" {
		return ""
	}
	base := filepath.Base(trimmedPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	prefix := strings.TrimSpace(filePrefix)
	if prefix != "" && strings.HasPrefix(name, prefix) {
		id := strings.TrimSpace(strings.TrimPrefix(name, prefix))
		if id != "" {
			return id
		}
	}
	return strings.TrimSpace(name)
}

func sanitizeLogToken(value, fallback string) string {
	if value == "" {
		return fallback
	}
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return '-'
		}
	}, value)
	clean = strings.Trim(clean, "-._")
	if clean == "" {
		return fallback
	}
	return clean
}

// ansiblePlaybookLineLogger は ansible-playbook の標準出力/標準エラーを1行ずつ slog.Info へ流し、
// 長時間実行中もCIログ等でリアルタイムに進捗を確認できるようにする。
type ansiblePlaybookLineLogger struct {
	resourceName string
	resourceID   string
	pending      []byte
}

func (l *ansiblePlaybookLineLogger) Write(p []byte) (int, error) {
	l.pending = append(l.pending, p...)
	for {
		idx := bytes.IndexByte(l.pending, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(string(l.pending[:idx]), "\r")
		l.pending = l.pending[idx+1:]
		if line == "" {
			continue
		}
		slog.Info("ansible-playbook progress", "resource", l.resourceName, "id", l.resourceID, "line", line)
	}
	return len(p), nil
}

func runAnsiblePlaybookWithLogging(args []string, resourceName, resourceID string) error {
	logPath, err := ansiblePlaybookLogPath(resourceName, resourceID)
	if err != nil {
		return fmt.Errorf("failed to prepare ansible log path: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open ansible log file %q: %w", logPath, err)
	}
	defer func() {
		_ = logFile.Close()
	}()

	_, _ = fmt.Fprintf(logFile, "\n=== %s ansible-playbook %s ===\n", time.Now().UTC().Format(time.RFC3339), strings.Join(args, " "))

	var buf bytes.Buffer
	lineLogger := &ansiblePlaybookLineLogger{resourceName: resourceName, resourceID: resourceID}
	mw := io.MultiWriter(&buf, logFile, lineLogger)

	cmd := exec.Command("ansible-playbook", args...)
	// PYTHONUNBUFFERED を指定しないと、パイプ接続時に ansible-playbook (Python) の出力がブロック単位で
	// バッファされ、実行完了までラインロガーへ何も流れない。
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	cmd.Stdout = mw
	cmd.Stderr = mw
	if err := cmd.Run(); err != nil {
		trimmed := strings.TrimSpace(buf.String())
		if trimmed == "" {
			return fmt.Errorf("ansible-playbook failed (log: %s): %w", logPath, err)
		}
		return fmt.Errorf("ansible-playbook failed (log: %s): %w: %s", logPath, err, trimmed)
	}

	return nil
}