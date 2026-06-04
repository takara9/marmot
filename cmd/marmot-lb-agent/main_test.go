package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireLockPreventsConcurrentExecution(t *testing.T) {
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, "agent.lock")

	first, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("failed to acquire first lock: %v", err)
	}
	defer releaseLock(first)

	second, err := acquireLock(lockPath)
	if err == nil {
		releaseLock(second)
		t.Fatalf("expected second lock acquisition to fail")
	}
}

func TestAcquireLockCanBeReacquiredAfterRelease(t *testing.T) {
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, "agent.lock")

	first, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("failed to acquire first lock: %v", err)
	}
	releaseLock(first)

	second, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("failed to reacquire lock after release: %v", err)
	}
	releaseLock(second)
}

func TestReconcileAppliesDesiredConfig(t *testing.T) {
	tmp := t.TempDir()
	desiredPath := filepath.Join(tmp, "haproxy-desired.cfg")
	activePath := filepath.Join(tmp, "haproxy.cfg")
	statePath := filepath.Join(tmp, "state.json")
	logPath := filepath.Join(tmp, "cmd.log")

	writeScript(t, filepath.Join(tmp, "haproxy"), "#!/bin/sh\necho haproxy $@ >> \"$MARMOT_TEST_LOG\"\nexit 0\n")
	writeScript(t, filepath.Join(tmp, "systemctl"), "#!/bin/sh\necho systemctl $@ >> \"$MARMOT_TEST_LOG\"\nexit 0\n")

	originalPath := os.Getenv("PATH")
	originalLog := os.Getenv("MARMOT_TEST_LOG")
	t.Setenv("PATH", tmp+":"+originalPath)
	t.Setenv("MARMOT_TEST_LOG", logPath)
	t.Cleanup(func() {
		_ = os.Setenv("PATH", originalPath)
		_ = os.Setenv("MARMOT_TEST_LOG", originalLog)
	})

	desired := "frontend fe\n  bind *:80\n"
	if err := os.WriteFile(desiredPath, []byte(desired), 0o644); err != nil {
		t.Fatalf("failed to write desired config: %v", err)
	}

	reconcile(desiredPath, activePath, statePath)

	active, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("failed to read active config: %v", err)
	}
	if string(active) != desired {
		t.Fatalf("unexpected active config: got=%q want=%q", string(active), desired)
	}

	state := loadState(statePath)
	if state.LastAppliedHash == "" {
		t.Fatalf("expected LastAppliedHash to be set")
	}
	if !state.LastAppliedAt.IsZero() && state.LastError != "" {
		t.Fatalf("unexpected LastError: %q", state.LastError)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read command log: %v", err)
	}
	logStr := string(logBytes)
	if !contains(logStr, "haproxy -c -f "+activePath+".candidate") {
		t.Fatalf("expected haproxy validation command in log, got: %s", logStr)
	}
	if !contains(logStr, "systemctl reload haproxy") {
		t.Fatalf("expected systemctl reload command in log, got: %s", logStr)
	}
}

func TestReconcileSkipsWhenHashUnchanged(t *testing.T) {
	tmp := t.TempDir()
	desiredPath := filepath.Join(tmp, "haproxy-desired.cfg")
	activePath := filepath.Join(tmp, "haproxy.cfg")
	statePath := filepath.Join(tmp, "state.json")
	logPath := filepath.Join(tmp, "cmd.log")

	writeScript(t, filepath.Join(tmp, "haproxy"), "#!/bin/sh\necho haproxy $@ >> \"$MARMOT_TEST_LOG\"\nexit 0\n")
	writeScript(t, filepath.Join(tmp, "systemctl"), "#!/bin/sh\necho systemctl $@ >> \"$MARMOT_TEST_LOG\"\nexit 0\n")

	originalPath := os.Getenv("PATH")
	originalLog := os.Getenv("MARMOT_TEST_LOG")
	t.Setenv("PATH", tmp+":"+originalPath)
	t.Setenv("MARMOT_TEST_LOG", logPath)
	t.Cleanup(func() {
		_ = os.Setenv("PATH", originalPath)
		_ = os.Setenv("MARMOT_TEST_LOG", originalLog)
	})

	desired := "frontend fe\n  bind *:80\n"
	if err := os.WriteFile(desiredPath, []byte(desired), 0o644); err != nil {
		t.Fatalf("failed to write desired config: %v", err)
	}

	reconcile(desiredPath, activePath, statePath)

	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatalf("failed to clear command log: %v", err)
	}

	reconcile(desiredPath, activePath, statePath)

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read command log: %v", err)
	}
	if len(logBytes) != 0 {
		t.Fatalf("expected no command executions for unchanged hash, got: %s", string(logBytes))
	}
}

func TestReconcileKeepsActiveConfigOnValidationFailure(t *testing.T) {
	tmp := t.TempDir()
	desiredPath := filepath.Join(tmp, "haproxy-desired.cfg")
	activePath := filepath.Join(tmp, "haproxy.cfg")
	statePath := filepath.Join(tmp, "state.json")
	logPath := filepath.Join(tmp, "cmd.log")

	writeScript(t, filepath.Join(tmp, "haproxy"), "#!/bin/sh\necho haproxy $@ >> \"$MARMOT_TEST_LOG\"\nexit 1\n")
	writeScript(t, filepath.Join(tmp, "systemctl"), "#!/bin/sh\necho systemctl $@ >> \"$MARMOT_TEST_LOG\"\nexit 0\n")

	originalPath := os.Getenv("PATH")
	originalLog := os.Getenv("MARMOT_TEST_LOG")
	t.Setenv("PATH", tmp+":"+originalPath)
	t.Setenv("MARMOT_TEST_LOG", logPath)
	t.Cleanup(func() {
		_ = os.Setenv("PATH", originalPath)
		_ = os.Setenv("MARMOT_TEST_LOG", originalLog)
	})

	if err := os.WriteFile(activePath, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("failed to write original active config: %v", err)
	}
	if err := os.WriteFile(desiredPath, []byte("candidate\n"), 0o644); err != nil {
		t.Fatalf("failed to write desired config: %v", err)
	}

	reconcile(desiredPath, activePath, statePath)

	active, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("failed to read active config: %v", err)
	}
	if string(active) != "original\n" {
		t.Fatalf("active config should remain unchanged on validation failure, got: %q", string(active))
	}

	state := loadState(statePath)
	if state.LastError == "" {
		t.Fatalf("expected LastError to be set on validation failure")
	}
	if !contains(state.LastError, "haproxy validation failed") {
		t.Fatalf("unexpected LastError: %q", state.LastError)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read command log: %v", err)
	}
	if contains(string(logBytes), "systemctl reload haproxy") {
		t.Fatalf("systemctl reload should not run on validation failure")
	}
}

func writeScript(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("failed to write script %s: %v", path, err)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestLoadStateInvalidJSONReturnsZeroValue(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state.json")
	if err := os.WriteFile(statePath, []byte("{"), 0o644); err != nil {
		t.Fatalf("failed to write invalid state: %v", err)
	}
	state := loadState(statePath)
	if state != (agentState{}) {
		b, _ := json.Marshal(state)
		t.Fatalf("expected zero value state, got: %s", string(b))
	}
}