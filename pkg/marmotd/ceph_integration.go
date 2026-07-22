package marmotd

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/ceph"
	"github.com/takara9/marmot/pkg/virt"
	"libvirt.org/go/libvirtxml"
)

type cephVolumeClient interface {
	CreateVolume(ctx context.Context, req ceph.VolumeRequest) error
	DeleteVolume(ctx context.Context, pool, image string) error
	StatVolume(ctx context.Context, pool, image string) (ceph.VolumeInfo, error)
	ListVolumes(ctx context.Context, pool string) ([]string, error)
	Cleanup() error
}

var newCephVolumeClient = func(cfg ceph.Config) (cephVolumeClient, error) {
	client, err := ceph.NewClient(cfg, nil)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func runtimeCephConfig() ceph.Config {
	cfg := CurrentConfig()
	poolByClass := make(map[string]string, len(cfg.CephPoolByClass))
	for class, pool := range cfg.CephPoolByClass {
		poolByClass[class] = pool
	}
	return ceph.Config{
		ConfFile:    effectiveCephConfPath(),
		KeyringFile: effectiveCephKeyringPath(),
		PoolByClass: poolByClass,
	}
}

func cephSecretUUIDForServer(serverID string) string {
	seed := "marmot-ceph-secret:" + strings.TrimSpace(serverID)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed)).String()
}

func normalizeCephServerID(serverID string) (string, error) {
	trimmed := strings.TrimSpace(serverID)
	if trimmed == "" {
		return "", fmt.Errorf("server id is required")
	}
	return trimmed, nil
}

func isCephVolume(volume api.Volume) bool {
	return volume.Spec.Type != nil && strings.EqualFold(strings.TrimSpace(*volume.Spec.Type), "ceph")
}

func resolveCephProviderVolumeIDForAttach(volume api.Volume, cfg ceph.Config) (string, error) {
	if volume.Status != nil && volume.Status.ProviderVolumeId != nil {
		providerVolumeID := strings.TrimSpace(*volume.Status.ProviderVolumeId)
		if providerVolumeID != "" {
			parts := strings.SplitN(providerVolumeID, "/", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				return "", fmt.Errorf("invalid ceph providerVolumeId: %s", providerVolumeID)
			}
			return providerVolumeID, nil
		}
	}

	req, err := ceph.MapVolumeToRequest(volume, cfg)
	if err != nil {
		return "", err
	}
	return req.ProviderVolumeID(), nil
}

func buildCephDiskSpec(volume api.Volume, serverID string, dev string, bus uint) (virt.DiskSpec, error) {
	cfg := runtimeCephConfig()
	if !CurrentConfig().CephEnabled {
		return virt.DiskSpec{}, fmt.Errorf("ceph is disabled")
	}
	resolvedServerID, err := normalizeCephServerID(serverID)
	if err != nil {
		return virt.DiskSpec{}, err
	}

	monitors, user, err := ceph.ParseConnectionFromConf(cfg.ConfFile, cfg.KeyringFile)
	if err != nil {
		return virt.DiskSpec{}, err
	}

	providerVolumeID, err := resolveCephProviderVolumeIDForAttach(volume, cfg)
	if err != nil {
		return virt.DiskSpec{}, err
	}

	secretUUID := cephSecretUUIDForServer(resolvedServerID)
	return virt.DiskSpec{
		Dev:            dev,
		Bus:            bus,
		Type:           "rbd",
		Src:            providerVolumeID,
		CephMonitors:   monitors,
		CephUser:       user,
		CephSecretUUID: secretUUID,
	}, nil
}

func resolveCephDeleteTarget(volume api.Volume, cfg ceph.Config) (pool, image string, err error) {
	if providerVolumeID := ""; volume.Status != nil && volume.Status.ProviderVolumeId != nil {
		providerVolumeID = strings.TrimSpace(*volume.Status.ProviderVolumeId)
		if providerVolumeID != "" {
			parts := strings.SplitN(providerVolumeID, "/", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				return "", "", fmt.Errorf("invalid ceph providerVolumeId: %s", providerVolumeID)
			}
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
		}
	}
	_ = cfg
	return "", "", fmt.Errorf("ceph providerVolumeId is required for delete")
}

func cephSecretSpecForServer(serverID string) (libvirtxml.Secret, error) {
	resolvedServerID, err := normalizeCephServerID(serverID)
	if err != nil {
		return libvirtxml.Secret{}, err
	}
	secretUUID := cephSecretUUIDForServer(resolvedServerID)
	usageName := "marmot-ceph-" + resolvedServerID
	return libvirtxml.Secret{
		UUID: secretUUID,
		Usage: &libvirtxml.SecretUsage{
			Type: "ceph",
			Name: usageName,
		},
	}, nil
}

func hasCephStorage(storage *[]api.Volume) bool {
	if storage == nil {
		return false
	}
	for _, vol := range *storage {
		if isCephVolume(vol) {
			return true
		}
	}
	return false
}

func prepareCephSecretForServer(l *virt.LibVirtEp, serverID string) error {
	runtimeCfg := CurrentConfig()
	if !runtimeCfg.CephEnabled {
		return nil
	}
	cephCfg := runtimeCephConfig()
	if _, err := normalizeCephServerID(serverID); err != nil {
		return err
	}
	if l == nil {
		return fmt.Errorf("libvirt endpoint is nil")
	}
	if strings.TrimSpace(cephCfg.KeyringFile) == "" {
		return fmt.Errorf("ceph keyring file is required")
	}
	secretSpec, err := cephSecretSpecForServer(serverID)
	if err != nil {
		return err
	}
	return l.EnsureCephSecret(secretSpec, cephCfg.KeyringFile)
}

func removeCephSecretForServer(l *virt.LibVirtEp, serverID string) error {
	if l == nil {
		return nil
	}
	resolvedServerID, err := normalizeCephServerID(serverID)
	if err != nil {
		return err
	}
	return l.RemoveCephSecretByUUID(cephSecretUUIDForServer(resolvedServerID))
}
