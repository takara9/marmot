package controller

import (
	"errors"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KubernetesEngineControlPlaneNAT", func() {
	var (
		origRunIPTables     = controlPlaneRunIPTables
		origRunSysctl       = controlPlaneRunSysctl
		origWriteFile       = controlPlaneWriteFile
		origEnableIPForward = controlPlaneEnableIPForward
	)

	AfterEach(func() {
		controlPlaneRunIPTables = origRunIPTables
		controlPlaneRunSysctl = origRunSysctl
		controlPlaneWriteFile = origWriteFile
		controlPlaneEnableIPForward = origEnableIPForward
	})

	It("rejects invalid inputs", func() {
		Expect(EnsureKubernetesEngineControlPlaneNAT("", "172.16.90.100", 6443)).To(HaveOccurred())
		Expect(EnsureKubernetesEngineControlPlaneNAT("not-an-ip", "172.16.90.100", 6443)).To(HaveOccurred())
		Expect(EnsureKubernetesEngineControlPlaneNAT("203.0.113.10", "not-an-ip", 6443)).To(HaveOccurred())
		Expect(EnsureKubernetesEngineControlPlaneNAT("203.0.113.10", "172.16.90.100", 0)).To(HaveOccurred())
	})

	It("enables ip_forward and adds DNAT/MASQUERADE rules when absent", func() {
		var iptablesCalls [][]string
		controlPlaneEnableIPForward = func() error { return nil }
		controlPlaneRunIPTables = func(args ...string) ([]byte, error) {
			iptablesCalls = append(iptablesCalls, append([]string(nil), args...))
			for _, a := range args {
				if a == "-C" {
					// -C (check) always reports "rule not found" so -A (add) is exercised.
					return nil, errors.New("rule not found")
				}
			}
			return nil, nil
		}

		Expect(EnsureKubernetesEngineControlPlaneNAT("203.0.113.10", "172.16.90.100", 6443)).To(Succeed())

		var sawDNATCheck, sawDNATAdd, sawMasqCheck, sawMasqAdd bool
		for _, call := range iptablesCalls {
			joined := strings.Join(call, " ")
			switch {
			case strings.Contains(joined, "-C") && strings.Contains(joined, "PREROUTING"):
				sawDNATCheck = true
			case strings.Contains(joined, "-A") && strings.Contains(joined, "PREROUTING"):
				sawDNATAdd = true
			case strings.Contains(joined, "-C") && strings.Contains(joined, "POSTROUTING"):
				sawMasqCheck = true
			case strings.Contains(joined, "-A") && strings.Contains(joined, "POSTROUTING"):
				sawMasqAdd = true
			}
		}
		Expect(sawDNATCheck).To(BeTrue())
		Expect(sawDNATAdd).To(BeTrue())
		Expect(sawMasqCheck).To(BeTrue())
		Expect(sawMasqAdd).To(BeTrue())
	})

	It("skips adding rules that already exist (idempotent)", func() {
		controlPlaneEnableIPForward = func() error { return nil }
		var addCalled bool
		controlPlaneRunIPTables = func(args ...string) ([]byte, error) {
			for _, a := range args {
				if a == "-A" {
					addCalled = true
				}
			}
			// -C succeeds: rule already present.
			return nil, nil
		}

		Expect(EnsureKubernetesEngineControlPlaneNAT("203.0.113.10", "172.16.90.100", 6443)).To(Succeed())
		Expect(addCalled).To(BeFalse())
	})

	It("removes existing DNAT/MASQUERADE rules in reverse order", func() {
		var deleteChains []string
		controlPlaneRunIPTables = func(args ...string) ([]byte, error) {
			for i, a := range args {
				if a == "-D" && i+1 < len(args) {
					deleteChains = append(deleteChains, args[i+1])
				}
			}
			// -C succeeds: rule present, so -D will be attempted.
			return nil, nil
		}

		Expect(RemoveKubernetesEngineControlPlaneNAT("203.0.113.10", "172.16.90.100", 6443)).To(Succeed())
		Expect(deleteChains).To(Equal([]string{"POSTROUTING", "PREROUTING"}))
	})

	It("skips removing rules that are already absent", func() {
		var deleteCalled bool
		controlPlaneRunIPTables = func(args ...string) ([]byte, error) {
			for _, a := range args {
				if a == "-D" {
					deleteCalled = true
				}
			}
			// -C always fails: rule absent.
			return nil, errors.New("rule not found")
		}

		Expect(RemoveKubernetesEngineControlPlaneNAT("203.0.113.10", "172.16.90.100", 6443)).To(Succeed())
		Expect(deleteCalled).To(BeFalse())
	})

	It("persists and applies net.ipv4.ip_forward=1", func() {
		var writtenPath string
		var writtenData []byte
		var writtenMode os.FileMode
		var sysctlArgs []string
		controlPlaneWriteFile = func(path string, data []byte, mode os.FileMode) error {
			writtenPath = path
			writtenData = data
			writtenMode = mode
			return nil
		}
		controlPlaneRunSysctl = func(args ...string) ([]byte, error) {
			sysctlArgs = append([]string(nil), args...)
			return nil, nil
		}

		Expect(enableControlPlaneIPForward()).To(Succeed())
		Expect(writtenPath).To(Equal(controlPlaneSysctlConfPath))
		Expect(string(writtenData)).To(Equal("net.ipv4.ip_forward=1\n"))
		Expect(writtenMode).To(Equal(os.FileMode(0o644)))
		Expect(sysctlArgs).To(Equal([]string{"-w", "net.ipv4.ip_forward=1"}))
	})
})
