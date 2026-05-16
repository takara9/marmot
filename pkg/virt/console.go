package virt

import (
	"fmt"
	"strings"

	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

// GetDomainConsolePath reads the active libvirt XML and returns the console device path.
func GetDomainConsolePath(dom *libvirt.Domain) (string, error) {
	if dom == nil {
		return "", fmt.Errorf("domain is nil")
	}
	xmlDesc, err := dom.GetXMLDesc(0)
	if err != nil {
		return "", err
	}
	return ExtractDomainConsolePath(xmlDesc)
}

// ExtractDomainConsolePath parses domain XML and returns the first usable console path.
func ExtractDomainConsolePath(xmlDesc string) (string, error) {
	var dom libvirtxml.Domain
	if err := dom.Unmarshal(xmlDesc); err != nil {
		return "", err
	}
	if dom.Devices == nil {
		return "", fmt.Errorf("console path not found in domain xml")
	}

	if path := firstSerialPath(dom.Devices.Serials); path != "" {
		return path, nil
	}
	if path := firstConsolePath(dom.Devices.Consoles); path != "" {
		return path, nil
	}

	return "", fmt.Errorf("console path not found in domain xml")
}

func firstSerialPath(items []libvirtxml.DomainSerial) string {
	for _, item := range items {
		if path := chardevSourcePath(item.Source); path != "" {
			return path
		}
	}
	return ""
}

func firstConsolePath(items []libvirtxml.DomainConsole) string {
	for _, item := range items {
		if path := chardevPath(item.TTY, item.Source); path != "" {
			return path
		}
	}
	return ""
}

func chardevSourcePath(source *libvirtxml.DomainChardevSource) string {
	if source == nil {
		return ""
	}
	if source.Pty != nil && strings.TrimSpace(source.Pty.Path) != "" {
		return strings.TrimSpace(source.Pty.Path)
	}
	if source.UNIX != nil && strings.TrimSpace(source.UNIX.Path) != "" {
		return strings.TrimSpace(source.UNIX.Path)
	}
	return ""
}

func chardevPath(tty string, source *libvirtxml.DomainChardevSource) string {
	if strings.TrimSpace(tty) != "" {
		return strings.TrimSpace(tty)
	}
	return chardevSourcePath(source)
}