package marmotd

import (
	"context"
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

type imageOSModule interface {
	key() string
	customizeDownloadedImage(ctx context.Context, qcowPath string) error
	applyFollowerSpec(follower *api.Image, head api.Image)
}

type commonImageOSModule struct {
	moduleKey string
}

func (m commonImageOSModule) key() string {
	return m.moduleKey
}

func (m commonImageOSModule) customizeDownloadedImage(ctx context.Context, qcowPath string) error {
	return customizeQcowImageWithContext(ctx, qcowPath)
}

func (m commonImageOSModule) applyFollowerSpec(follower *api.Image, head api.Image) {
	if follower == nil {
		return
	}
	follower.Spec.Kind = util.StringPtr("os")
	follower.Spec.Type = util.StringPtr("qcow2")
	follower.Spec.SourceUrl = nil

	if head.Spec.Kind != nil {
		follower.Spec.Kind = util.StringPtr(strings.TrimSpace(*head.Spec.Kind))
	}
	if head.Spec.Type != nil {
		follower.Spec.Type = util.StringPtr(strings.TrimSpace(*head.Spec.Type))
	}
	if head.Spec.SourceUrl != nil {
		sourceURL := strings.TrimSpace(*head.Spec.SourceUrl)
		if sourceURL != "" {
			follower.Spec.SourceUrl = util.StringPtr(sourceURL)
		}
	}
	if head.Spec.OsName != nil {
		osName := strings.TrimSpace(*head.Spec.OsName)
		if osName != "" {
			follower.Spec.OsName = util.StringPtr(osName)
		}
	}
	if head.Spec.OsVersion != nil {
		osVersion := strings.TrimSpace(*head.Spec.OsVersion)
		if osVersion != "" {
			follower.Spec.OsVersion = util.StringPtr(osVersion)
		}
	}
	if head.Spec.Size != nil {
		follower.Spec.Size = util.IntPtrInt(*head.Spec.Size)
	}
}

var (
	imageOSModuleUbuntu2204 = commonImageOSModule{moduleKey: "ubuntu22.04"}
	imageOSModuleUbuntu2404 = commonImageOSModule{moduleKey: "ubuntu24.04"}
	imageOSModuleUbuntu     = commonImageOSModule{moduleKey: "ubuntu"}
	imageOSModuleAlpine323  = commonImageOSModule{moduleKey: "alpine3.23"}
)

func resolveImageOSModuleFromImage(img api.Image) (imageOSModule, error) {
	osName := strings.TrimSpace(util.OrDefault(img.Spec.OsName, ""))
	osVersion := strings.TrimSpace(util.OrDefault(img.Spec.OsVersion, ""))
	return resolveImageOSModuleFromSpec(osName, osVersion)
}

func resolveImageOSModuleFromSpec(osName, osVersion string) (imageOSModule, error) {
	name := strings.ToLower(strings.TrimSpace(osName))
	version := strings.TrimSpace(osVersion)

	switch name {
	case "ubuntu":
		switch version {
		case "22.04":
			return imageOSModuleUbuntu2204, nil
		case "24.04":
			return imageOSModuleUbuntu2404, nil
		default:
			return imageOSModuleUbuntu, nil
		}
	case "alpine":
		if version == "3.23" {
			return imageOSModuleAlpine323, nil
		}
		return nil, fmt.Errorf("unsupported alpine image module version: %s", version)
	case "":
		return imageOSModuleUbuntu2204, nil
	default:
		return nil, fmt.Errorf("unsupported image module os: name=%q version=%q", osName, osVersion)
	}
}

// ApplyFollowerImageSpecByOS applies OS-specific follower spec mapping from a head image.
func ApplyFollowerImageSpecByOS(follower *api.Image, head api.Image) (string, error) {
	module, err := resolveImageOSModuleFromImage(head)
	if err != nil {
		return "", err
	}
	module.applyFollowerSpec(follower, head)
	return module.key(), nil
}
