package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDelCommandAllowsMultipleServerNameArgs(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	configPath := filepath.Join(homeDir, ".marmot")
	configBody := "current: 0\nendpoints:\n  - http://127.0.0.1:1\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0600); err != nil {
		t.Fatalf("failed to write client config: %v", err)
	}

	err := delCmd.RunE(delCmd, []string{"server", "biz,", "rest1,", "rest2"})
	if err == nil {
		t.Fatalf("expected command to continue past arg validation")
	}

	if strings.Contains(err.Error(), "del requires RESOURCE and NAME unless -f is specified") {
		t.Fatalf("server multi-name args were rejected by initial validation: %v", err)
	}

	if strings.Contains(err.Error(), "del requires exactly one NAME") {
		t.Fatalf("server multi-name args were rejected by single-name validation: %v", err)
	}
}
