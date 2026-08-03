package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestVersionPrintsClientVersionWhenServerUnreachable(t *testing.T) {
	tmp, err := os.CreateTemp("", "mactl-version-config-*.yaml")
	if err != nil {
		t.Fatalf("CreateTemp() failed: %v", err)
	}
	defer func() {
		if err := os.Remove(tmp.Name()); err != nil && !os.IsNotExist(err) {
			t.Errorf("Remove() failed: %v", err)
		}
	}()

	if _, err := tmp.WriteString("current: 0\nendpoints:\n  - http://127.0.0.1:1\n"); err != nil {
		t.Fatalf("WriteString() failed: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	oldAPI := apiConfigFilename
	oldOutput := outputStyle
	apiConfigFilename = tmp.Name()
	outputStyle = "text"
	defer func() {
		apiConfigFilename = oldAPI
		outputStyle = oldOutput
	}()

	out, stderr, runErr := captureStdoutAndStderr(func() error {
		return versionCmd.RunE(versionCmd, nil)
	})
	if runErr != nil {
		t.Fatalf("versionCmd.RunE() returned error: %v", runErr)
	}

	if !strings.Contains(out, "Client version =") {
		t.Fatalf("stdout does not contain client version. stdout=%q", out)
	}
	if !strings.Contains(stderr, "failed to get server version:") {
		t.Fatalf("stderr does not contain server failure message. stderr=%q", stderr)
	}
}

func captureStdoutAndStderr(fn func() error) (stdout string, stderr string, runErr error) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return "", "", err
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		return "", "", err
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	runErr = fn()

	_ = stdoutW.Close()
	_ = stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var outBuf bytes.Buffer
	_, _ = io.Copy(&outBuf, stdoutR)
	_ = stdoutR.Close()

	var errBuf bytes.Buffer
	_, _ = io.Copy(&errBuf, stderrR)
	_ = stderrR.Close()

	return outBuf.String(), errBuf.String(), runErr
}
