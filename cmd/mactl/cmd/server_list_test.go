package cmd

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

var _ = Describe("formatServerListText", func() {
	It("prints additional network interfaces on continuation lines", func() {
		name := "test-server-40"
		nodeName := "marmot1"
		cpu := 1
		memory := 2048
		addr1 := "192.168.100.2"
		addr2 := "192.168.1.71"

		output := formatServerListText([]api.Server{{
			Id: "3f738",
			Metadata: &api.Metadata{
				Name:     &name,
				NodeName: &nodeName,
			},
			Spec: &api.ServerSpec{
				Cpu:    &cpu,
				Memory: &memory,
				NetworkInterface: &[]api.NetworkInterface{
					{Address: &addr1, Networkname: "test-net-4"},
					{Address: &addr2, Networkname: "host-bridge"},
					{Networkname: "default"},
				},
			},
			Status: &api.Status{StatusCode: int(db.SERVER_RUNNING)},
		}})

		lines := strings.Split(strings.TrimSpace(output), "\n")
		Expect(lines).To(HaveLen(4), output)
		Expect(lines[0]).To(ContainSubstring("IP-ADDRESS"), output)
		Expect(lines[1]).To(ContainSubstring("3f738"), output)
		Expect(lines[1]).To(ContainSubstring("192.168.100.2"), output)
		Expect(lines[1]).To(ContainSubstring("test-net-4"), output)
		Expect(lines[2]).NotTo(ContainSubstring("3f738"), output)
		Expect(lines[2]).To(ContainSubstring("192.168.1.71"), output)
		Expect(lines[2]).To(ContainSubstring("host-bridge"), output)
		Expect(lines[3]).To(ContainSubstring("N/A"), output)
		Expect(lines[3]).To(ContainSubstring("default"), output)
	})

	It("falls back to N/A when no network interfaces are attached", func() {
		name := "test-server-41"
		cpu := 2
		memory := 4096

		output := formatServerListText([]api.Server{{
			Id: "592a2",
			Metadata: &api.Metadata{
				Name: &name,
			},
			Spec: &api.ServerSpec{
				Cpu:    &cpu,
				Memory: &memory,
			},
			Status: &api.Status{StatusCode: int(db.SERVER_RUNNING)},
		}})

		lines := strings.Split(strings.TrimSpace(output), "\n")
		Expect(lines).To(HaveLen(2), output)
		Expect(lines[1]).To(ContainSubstring("592a2"), output)
		Expect(lines[1]).To(ContainSubstring("N/A"), output)
	})

	It("prefixes ID with * when DeletionTimeStamp is set", func() {
		name := "test-server-deleting"
		now := time.Now()

		output := formatServerListText([]api.Server{{
			Id: "a1b2c",
			Metadata: &api.Metadata{
				Name: &name,
			},
			Status: &api.Status{
				StatusCode:         int(db.SERVER_DELETING),
				DeletionTimeStamp: &now,
			},
		}})

		lines := strings.Split(strings.TrimSpace(output), "\n")
		Expect(lines).To(HaveLen(2), output)
		Expect(lines[1]).To(ContainSubstring("*a1b2c"), output)
	})
})