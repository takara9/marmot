package marmotd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebPackageIncludesGatewayAssets(t *testing.T) {
	if os.Getenv("MARMOT_RUN_PACKAGE_E2E") != "1" {
		t.Skip("set MARMOT_RUN_PACKAGE_E2E=1 to run deb packaging e2e test")
	}
	if _, err := exec.LookPath("dpkg-deb"); err != nil {
		t.Skip("dpkg-deb command not found")
	}
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make command not found")
	}

	repoRoot := findRepoRootForTest(t)
	cmd := exec.Command("make", "package")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make package failed: %v\n%s", err, string(output))
	}

	tagBytes, err := os.ReadFile(filepath.Join(repoRoot, "TAG"))
	if err != nil {
		t.Fatalf("ReadFile(TAG) failed: %v", err)
	}
	debPath := filepath.Join(repoRoot, "dist", "marmot_v"+strings.TrimSpace(string(tagBytes))+"_amd64.deb")
	if _, err := os.Stat(debPath); err != nil {
		t.Fatalf("built deb not found: %v", err)
	}

	listOut, err := exec.Command("dpkg-deb", "-c", debPath).CombinedOutput()
	if err != nil {
		t.Fatalf("dpkg-deb -c failed: %v\n%s", err, string(listOut))
	}
	listText := string(listOut)
	for _, want := range []string{
		"./usr/local/marmot/gateway-playbooks/gateway-iptables.yaml.tmpl",
		"./var/lib/marmot/ansible-playbooks/templates/",
		"./etc/marmot/keys/",
	} {
		if !strings.Contains(listText, want) {
			t.Fatalf("package listing missing %q\n%s", want, listText)
		}
	}

	controlDir := t.TempDir()
	controlOut, err := exec.Command("dpkg-deb", "-e", debPath, controlDir).CombinedOutput()
	if err != nil {
		t.Fatalf("dpkg-deb -e failed: %v\n%s", err, string(controlOut))
	}
	postinstBytes, err := os.ReadFile(filepath.Join(controlDir, "postinst"))
	if err != nil {
		t.Fatalf("ReadFile(postinst) failed: %v", err)
	}
	postinst := string(postinstBytes)
	for _, want := range []string{
		"/var/lib/marmot/ansible-playbooks",
		"ssh-keygen -t rsa -b 4096",
		"/usr/local/marmot/gateway-playbooks",
	} {
		if !strings.Contains(postinst, want) {
			t.Fatalf("postinst missing %q\n%s", want, postinst)
		}
	}
}

func findRepoRootForTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("repository root not found from %s", wd)
	return ""
}
