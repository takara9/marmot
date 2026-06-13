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
		return fmt.Errorf("invalid spec.osName: %q (allowed: alpine, ubuntu, rockey, debian)", osName)
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
		"rockey": {
			"8": {},
			"9": {},
		},
		"debian": {
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
	case "rockey":
		return "rockey"
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
