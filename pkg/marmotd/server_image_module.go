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
	GenerateCloudInitISO(path, password, sshKey string, usernames []string, ansible *api.ServerAnsible, instanceID string) (string, error)
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

func (m commonServerImageModule) GenerateCloudInitISO(path, password, sshKey string, usernames []string, ansible *api.ServerAnsible, instanceID string) (string, error) {
	return GenerateCloudInitISO(path, password, sshKey, usernames, ansible, instanceID)
}

var (
	serverImageModuleUbuntu2204 = commonServerImageModule{key: "ubuntu22.04"}
	serverImageModuleUbuntu2404 = commonServerImageModule{key: "ubuntu24.04"}
	serverImageModuleUbuntu     = commonServerImageModule{key: "ubuntu"}
	serverImageModuleAlpine323  = commonServerImageModule{key: "alpine3.23", setupBootVolumeFn: util.SetupAlpineLinux}
	serverImageModuleRocky8     = commonServerImageModule{key: "rocky8", setupBootVolumeFn: util.SetupRockyLinux}
	serverImageModuleRocky9     = commonServerImageModule{key: "rocky9", setupBootVolumeFn: util.SetupRockyLinux}
	serverImageModuleAlmaLinux8 = commonServerImageModule{key: "almalinux8", setupBootVolumeFn: util.SetupAlmaLinux}
	serverImageModuleAlmaLinux9 = commonServerImageModule{key: "almalinux9", setupBootVolumeFn: util.SetupAlmaLinux}
	// Debian 12/13 は Ubuntu と同じ方式(netplan)でブートボリュームを初期化するため、
	// setupBootVolumeFn は指定せず util.SetupLinux にフォールバックさせる。
	serverImageModuleDebian12 = commonServerImageModule{key: "debian12"}
	serverImageModuleDebian13 = commonServerImageModule{key: "debian13"}
)

func normalizeServerImageDefault(server *api.Server) {
	server.NormalizeMMImageAlias()
	if server.Spec.MmImage == nil || strings.TrimSpace(*server.Spec.MmImage) == "" {
		server.Spec.MmImage = util.StringPtr("ubuntu24.04")
		server.NormalizeMMImageAlias()
	}
}

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

	// rockey は rocky の旧表記。既存データとの互換のためエイリアスとして扱う。
	if name == "rockey" {
		name = "rocky"
	}

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
	case "rocky":
		switch version {
		case "8":
			return serverImageModuleRocky8, nil
		case "9":
			return serverImageModuleRocky9, nil
		default:
			return nil, fmt.Errorf("unsupported rocky version: %s", version)
		}
	case "almalinux":
		switch version {
		case "8":
			return serverImageModuleAlmaLinux8, nil
		case "9":
			return serverImageModuleAlmaLinux9, nil
		default:
			return nil, fmt.Errorf("unsupported almalinux version: %s", version)
		}
	case "debian":
		switch version {
		case "12":
			return serverImageModuleDebian12, nil
		case "13":
			return serverImageModuleDebian13, nil
		default:
			return nil, fmt.Errorf("unsupported debian version: %s", version)
		}
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
	case strings.HasPrefix(v, "rocky8"), strings.HasPrefix(v, "rockey8"):
		// rockey8 は rocky8 の旧表記(互換維持のため受け付ける)。
		return "rocky", "8"
	case strings.HasPrefix(v, "rocky9"), strings.HasPrefix(v, "rockey9"):
		// rockey9 は rocky9 の旧表記(互換維持のため受け付ける)。
		return "rocky", "9"
	case strings.HasPrefix(v, "almalinux8"):
		return "almalinux", "8"
	case strings.HasPrefix(v, "almalinux9"):
		return "almalinux", "9"
	case strings.HasPrefix(v, "debian12"):
		return "debian", "12"
	case strings.HasPrefix(v, "debian13"):
		return "debian", "13"
	default:
		return "", ""
	}
}
