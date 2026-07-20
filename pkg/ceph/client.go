package ceph

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const (
	DefaultCephConfFile    = "/etc/ceph/ceph.conf"
	DefaultCephKeyringFile = "/etc/ceph/ceph.client.admin.keyring"
)

func defaultCephConfFile() string {
	if path := strings.TrimSpace(os.Getenv("MARMOT_CEPH_CONF_FILE")); path != "" {
		return path
	}
	return DefaultCephConfFile
}

func defaultCephKeyringFile() string {
	if path := strings.TrimSpace(os.Getenv("MARMOT_CEPH_KEYRING_FILE")); path != "" {
		return path
	}
	return DefaultCephKeyringFile
}

var defaultPoolByClass = map[string]string{
	"hdd":  "marmot-hdd",
	"ssd":  "marmot-ssd",
	"nvme": "marmot-nvme",
}

type VolumeClient interface {
	CreateVolume(ctx context.Context, req VolumeRequest) error
	DeleteVolume(ctx context.Context, pool, image string) error
	StatVolume(ctx context.Context, pool, image string) (VolumeInfo, error)
	ListVolumes(ctx context.Context, pool string) ([]string, error)
}

type Config struct {
	ConfFile    string
	KeyringFile string
	PoolByClass map[string]string
}

type Client struct {
	cfg    Config
	runner Runner
}

func DefaultConfig() Config {
	return Config{
		ConfFile:    defaultCephConfFile(),
		KeyringFile: defaultCephKeyringFile(),
		PoolByClass: map[string]string{
			"hdd":  defaultPoolByClass["hdd"],
			"ssd":  defaultPoolByClass["ssd"],
			"nvme": defaultPoolByClass["nvme"],
		},
	}
}

func NewClient(cfg Config, runner Runner) (*Client, error) {
	normalized := normalizeConfig(cfg)
	if strings.TrimSpace(normalized.ConfFile) == "" {
		return nil, fmt.Errorf("ceph conf file is required")
	}
	if strings.TrimSpace(normalized.KeyringFile) == "" {
		return nil, fmt.Errorf("ceph keyring file is required")
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Client{cfg: normalized, runner: runner}, nil
}

func (c Config) Cleanup() error {
	return nil
}

func (c *Client) Cleanup() error {
	if c == nil {
		return nil
	}
	return c.cfg.Cleanup()
}

func (c Config) PoolForStorageClass(storageClass string) (string, error) {
	key := normalizeStorageClass(storageClass)
	if key == "" {
		return "", fmt.Errorf("storageClass is required")
	}
	pool, ok := c.PoolByClass[key]
	if !ok || strings.TrimSpace(pool) == "" {
		return "", fmt.Errorf("unsupported storageClass %q", storageClass)
	}
	return strings.TrimSpace(pool), nil
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.ConfFile) == "" {
		cfg.ConfFile = defaultCephConfFile()
	}
	if strings.TrimSpace(cfg.KeyringFile) == "" {
		cfg.KeyringFile = defaultCephKeyringFile()
	}
	if len(cfg.PoolByClass) == 0 {
		cfg.PoolByClass = map[string]string{
			"hdd":  defaultPoolByClass["hdd"],
			"ssd":  defaultPoolByClass["ssd"],
			"nvme": defaultPoolByClass["nvme"],
		}
	}
	return cfg
}
