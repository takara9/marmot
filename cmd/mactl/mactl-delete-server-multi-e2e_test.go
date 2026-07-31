package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

var _ = Describe("MactlDeleteServerMultiE2E", Ordered, func() {
	var mockServer *mockServerHandle
	var containerID string
	var testHomeDir string

	BeforeAll(func(specCtx SpecContext) {
		cleanupTestEnvironment()
		var err error
		testHomeDir, err = setupMactlTestHome()
		Expect(err).NotTo(HaveOccurred())
		if err := ensureMactlTestBinary(); err != nil {
			Fail(fmt.Sprintf("Failed to build mactl test binary: %v", err))
		}

		var etcdEp string
		containerID, etcdEp, err = startEtcdContainer()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %v", err))
		}

		mockServer, err = startMockServer(etcdEp, "testdata/marmotd.json")
		Expect(err).NotTo(HaveOccurred())
		Expect(loginAsAdmin()).NotTo(HaveOccurred())
	})

	AfterAll(func(specCtx SpecContext) {
		if mockServer != nil {
			mockServer.Stop()
		}

		if strings.TrimSpace(containerID) != "" {
			cmd := exec.Command("docker", "kill", containerID)
			_, _ = cmd.CombinedOutput()
			cmd = exec.Command("docker", "rm", containerID)
			_, _ = cmd.CombinedOutput()
		}

		if strings.TrimSpace(testHomeDir) != "" {
			_ = os.RemoveAll(testHomeDir)
		}
		_ = os.Remove("bin/mactl-test")
		_ = os.Remove("/var/actions-runner/_work/marmot/marmot/cmd/mactl/bin/mactl-test")
		cleanupTestEnvironment()
	})

	It("mactl delete server biz,rest1,rest2,rest3,db で一括削除できる", func() {
		names := []string{"biz", "rest1", "rest2", "rest3", "db"}
		tempDir, err := os.MkdirTemp("", "mactl-issue603-")
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			err := os.RemoveAll(tempDir)
			Expect(err).NotTo(HaveOccurred())
		}()

		createdIDs := make(map[string]string, len(names))
		for _, name := range names {
			yamlPath := fmt.Sprintf("%s/%s.yaml", tempDir, name)
			yamlBody := fmt.Sprintf(`apiVersion: v1
kind: Server
metadata:
    name: %s
    comment: "issue-603 e2e"
spec:
    cpu: 1
    memory: 1024
`, name)
			err = os.WriteFile(yamlPath, []byte(yamlBody), 0644)
			Expect(err).NotTo(HaveOccurred())

			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "create", "--configfile", yamlPath, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "create failed for %s: %s", name, string(stdoutStderr))

			var reply api.Success
			err = json.Unmarshal(stdoutStderr, &reply)
			Expect(err).NotTo(HaveOccurred(), "unexpected create output for %s: %s", name, string(stdoutStderr))
			createdIDs[name] = reply.Id
		}

		Eventually(func(g Gomega) {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			g.Expect(err).NotTo(HaveOccurred())

			var servers []api.Server
			err = json.Unmarshal(stdoutStderr, &servers)
			g.Expect(err).NotTo(HaveOccurred())

			found := map[string]bool{}
			for _, server := range servers {
				found[server.Metadata.Name] = true
			}
			for _, name := range names {
				g.Expect(found[name]).To(BeTrue(), "server %s not found in list", name)
			}
		}, 120*time.Second, 5*time.Second).Should(Succeed())

		cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "delete", "server", "biz,rest1,rest2,rest3,db")
		stdoutStderr, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "bulk delete failed: %s", string(stdoutStderr))

		for _, name := range names {
			serverID := createdIDs[name]
			Eventually(func(g Gomega) {
				detailCmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverID, "--output", "json")
				stdoutStderr, err := detailCmd.CombinedOutput()

				if err != nil {
					body := string(stdoutStderr)
					if strings.Contains(strings.ToLower(body), "not found") || strings.Contains(body, "IDが存在しません") {
						return
					}
				}
				g.Expect(err).NotTo(HaveOccurred(), "detail failed for %s(id=%s): %s", name, serverID, string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_DELETING)))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		}

		Eventually(func(g Gomega) {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			g.Expect(err).NotTo(HaveOccurred())

			var servers []api.Server
			err = json.Unmarshal(stdoutStderr, &servers)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(len(servers)).To(Equal(0))
		}, 120*time.Second, 5*time.Second).Should(Succeed())
	})
})
