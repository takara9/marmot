package marmotd

import (
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

type serverImageModule interface {
	Key() string
	SetupBootVolume(spec api.Server) error
	GenerateCloudInitISO(path, password, sshKey string, usernames []string) (string, error)
}

type commonServerImageModule struct {
	key               string
	setupBootVolumeFn func(spec api.Server) error
}

func (m commonServerImageModule) Key() string {
	return m.key
}

func (m commonServerImageModule) SetupBootVolume(spec api.Server) error {
	if m.setupBootVolumeFn != nil {
		return m.setupBootVolumeFn(spec)
	}
	return util.SetupLinux(spec)
}

func (m commonServerImageModule) GenerateCloudInitISO(path, password, sshKey string, usernames []string) (string, error) {
	return GenerateCloudInitISO(path, password, sshKey, usernames)
}

var (
	serverImageModuleUbuntu2204 = commonServerImageModule{key: "ubuntu22.04"}
	serverImageModuleUbuntu2404 = commonServerImageModule{key: "ubuntu24.04"}
	serverImageModuleUbuntu     = commonServerImageModule{key: "ubuntu"}
	serverImageModuleAlpine323  = commonServerImageModule{key: "alpine3.23", setupBootVolumeFn: util.SetupAlpineLinux}
)

func resolveServerImageModule(m *Marmot, bootVol api.Volume) (serverImageModule, error) {
	osName, osVersion := "", ""
	if img, err := resolveImageTemplateByVolumeNode(m, bootVol); err == nil {
		if img.Spec.OsName != nil {
			osName = strings.TrimSpace(*img.Spec.OsName)
		}
		if img.Spec.OsVersion != nil {
			osVersion = strings.TrimSpace(*img.Spec.OsVersion)
		}
	}

	if osName == "" || osVersion == "" {
		variant := ""
		if bootVol.Spec.OsVariant != nil {
			variant = strings.TrimSpace(*bootVol.Spec.OsVariant)
		}
		vName, vVersion := deriveOSFromVariant(variant)
		if osName == "" {
			osName = vName
		}
		if osVersion == "" {
			osVersion = vVersion
		}
	}

	module, err := resolveServerImageModuleFromOS(osName, osVersion)
	if err != nil {
		return nil, err
	}
	return module, nil
}

func resolveServerImageModuleFromOS(osName, osVersion string) (serverImageModule, error) {
	name := strings.ToLower(strings.TrimSpace(osName))
	version := strings.TrimSpace(osVersion)

	switch name {
	case "ubuntu":
		switch version {
		case "22.04":
			return serverImageModuleUbuntu2204, nil
		case "24.04":
			return serverImageModuleUbuntu2404, nil
		default:
			return serverImageModuleUbuntu, nil
		}
	case "alpine":
		if version == "3.23" {
			return serverImageModuleAlpine323, nil
		}
		return nil, fmt.Errorf("unsupported alpine version: %s", version)
	case "":
		return serverImageModuleUbuntu2204, nil
	default:
		return nil, fmt.Errorf("unsupported image os: name=%q version=%q", osName, osVersion)
	}
}

func deriveOSFromVariant(osVariant string) (string, string) {
	v := strings.ToLower(strings.TrimSpace(osVariant))
	switch {
	case strings.HasPrefix(v, "ubuntu22.04"):
		return "ubuntu", "22.04"
	case strings.HasPrefix(v, "ubuntu24.04"):
		return "ubuntu", "24.04"
	case strings.HasPrefix(v, "alpine3.23"):
		return "alpine", "3.23"
	default:
		return "", ""
	}
}
