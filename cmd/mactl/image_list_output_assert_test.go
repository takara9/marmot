package main

import (
	"strings"

	. "github.com/onsi/gomega"
)

func assertImageListTextHeader(output []byte) {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	headerLine := ""
	for _, line := range lines {
		if strings.Contains(line, "IMAGE-ID") {
			headerLine = line
			break
		}
	}

	Expect(headerLine).NotTo(BeEmpty(), "image list text output header was not found")
	Expect(headerLine).NotTo(ContainSubstring("LV-PATH"))
	Expect(headerLine).NotTo(ContainSubstring("QCOW2-PATH"))
	Expect(strings.Fields(headerLine)).To(Equal([]string{
		"No",
		"IMAGE-ID",
		"IMAGE-NAME",
		"STATUS",
		"NODE-NAME",
		"ROLE",
		"LV",
		"QCOW2",
		"CREATED-AT",
	}))
}
