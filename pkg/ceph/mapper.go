package ceph

import (
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
)

func MapVolumeToRequest(volume api.Volume, cfg Config) (VolumeRequest, error) {
	if volume.Spec.Type == nil || !strings.EqualFold(strings.TrimSpace(*volume.Spec.Type), "ceph") {
		return VolumeRequest{}, fmt.Errorf("volume type must be ceph")
	}
	if volume.Spec.Kind != nil && !strings.EqualFold(strings.TrimSpace(*volume.Spec.Kind), "data") {
		return VolumeRequest{}, fmt.Errorf("ceph volume kind must be data in phase 1")
	}
	if volume.Spec.Size == nil || *volume.Spec.Size < 1 {
		return VolumeRequest{}, fmt.Errorf("ceph volume size must be at least 1GB")
	}
	if api.VolumeID(volume) == "" {
		return VolumeRequest{}, fmt.Errorf("volume id is required")
	}

	storageClass := normalizeStorageClass(ptrString(volume.Spec.StorageClass))
	if err := ValidateStorageClass(storageClass); err != nil {
		return VolumeRequest{}, err
	}
	pool, err := normalizeConfig(cfg).PoolForStorageClass(storageClass)
	if err != nil {
		return VolumeRequest{}, err
	}

	return VolumeRequest{
		Pool:         pool,
		Image:        imageNameFromVolumeID(api.VolumeID(volume)),
		SizeGB:       *volume.Spec.Size,
		StorageClass: storageClass,
	}, nil
}

func ValidateStorageClass(storageClass string) error {
	switch normalizeStorageClass(storageClass) {
	case "hdd", "ssd", "nvme":
		return nil
	case "":
		return fmt.Errorf("storageClass is required for ceph volumes")
	default:
		return fmt.Errorf("storageClass must be one of hdd, ssd, nvme")
	}
}

func normalizeStorageClass(storageClass string) string {
	return strings.ToLower(strings.TrimSpace(storageClass))
}

func imageNameFromVolumeID(volumeID string) string {
	trimmed := strings.TrimSpace(volumeID)
	if strings.HasPrefix(trimmed, "vol-") {
		return trimmed
	}
	return "vol-" + trimmed
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}