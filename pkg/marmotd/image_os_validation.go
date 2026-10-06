package marmotd

import (
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
)

// validateImageOSSpec validates allowed values for spec.osName/spec.osVersion.
func validateImageOSSpec(spec *api.ImageSpec) error {
	if spec == nil {
		return nil
	}

	osName := strings.TrimSpace(derefString(spec.OsName))
	osVersion := strings.TrimSpace(derefString(spec.OsVersion))

	if osName == "" && osVersion == "" {
		return nil
	}
	if osName == "" {
		return fmt.Errorf("spec.osName is required when spec.osVersion is set")
	}
	if osVersion == "" {
		return fmt.Errorf("spec.osVersion is required when spec.osName is set")
	}
	if osName != strings.ToLower(osName) {
		return fmt.Errorf("spec.osName must be lowercase")
	}

	canonical := canonicalOSName(osName)
	if canonical == "" {
		return fmt.Errorf("invalid spec.osName: %q (allowed: alpine, ubuntu, rocky, rockey, almalinux, debian)", osName)
	}

	allowedVersions := map[string]map[string]struct{}{
		"alpine": {
			"3.23": {},
		},
		"ubuntu": {
			"22.04": {},
			"24.04": {},
			"26.04": {},
		},
		// rockey は rocky の正式化前の旧表記(互換維持のため canonicalOSName で rocky へ正規化する)。
		"rocky": {
			"8": {},
			"9": {},
		},
		"almalinux": {
			"8": {},
			"9": {},
		},
		"debian": {
			"12": {},
			"13": {},
		},
	}

	if _, ok := allowedVersions[canonical][osVersion]; !ok {
		return fmt.Errorf("invalid spec.osVersion %q for spec.osName %q", osVersion, osName)
	}

	return nil
}

func canonicalOSName(name string) string {
	switch name {
	case "alpine":
		return "alpine"
	case "ubuntu":
		return "ubuntu"
	case "rocky", "rockey":
		// rockey は既存データとの互換のために受け付ける rocky の旧表記。
		return "rocky"
	case "almalinux":
		return "almalinux"
	case "debian":
		return "debian"
	default:
		return ""
	}
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
