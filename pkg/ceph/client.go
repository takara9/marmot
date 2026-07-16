package ceph

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func defaultMonitorHost() string {
	return strings.TrimSpace(os.Getenv("CEPH_IPADDR"))
}

func defaultCephAuth() (string, string) {
	secret := strings.TrimSpace(os.Getenv("CEPH_POOL_KEY"))
	if secret == "" {
		return "", ""
	}

	user := parseKeyringUser(secret)
	f, err := os.CreateTemp("", "marmot-ceph-pool-*.keyring")
	if err != nil {
		return user, ""
	}
	keyFile := f.Name()
	if _, writeErr := f.WriteString(ensureTrailingNewline(secret)); writeErr != nil {
		_ = f.Close()
		_ = os.Remove(keyFile)
		return user, ""
	}
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(keyFile)
		return user, ""
	}
	if chmodErr := os.Chmod(keyFile, 0o600); chmodErr != nil {
		_ = os.Remove(keyFile)
		return user, ""
	}
	return user, keyFile
}

func parseKeyringUser(secret string) string {
	for _, line := range strings.Split(secret, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
			continue
		}
		section := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
		return strings.TrimPrefix(section, "client.")
	}
	return ""
}

func ensureTrailingNewline(value string) string {
	if strings.HasSuffix(value, "\n") {
		return value
	}
	return value + "\n"
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
	Monitors    []string
	User        string
	KeyFile     string
	PoolByClass map[string]string
}

type Client struct {
	cfg    Config
	runner Runner
}

func DefaultConfig() Config {
	user, keyFile := defaultCephAuth()
	return Config{
		Monitors: []string{defaultMonitorHost()},
		User:     user,
		KeyFile:  keyFile,
		PoolByClass: map[string]string{
			"hdd":  defaultPoolByClass["hdd"],
			"ssd":  defaultPoolByClass["ssd"],
			"nvme": defaultPoolByClass["nvme"],
		},
	}
}

func NewClient(cfg Config, runner Runner) (*Client, error) {
	normalized := normalizeConfig(cfg)
	if normalized.MonitorHosts() == "" {
		return nil, fmt.Errorf("ceph monitors are required")
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Client{cfg: normalized, runner: runner}, nil
}

func (c Config) MonitorHosts() string {
	monitors := make([]string, 0, len(c.Monitors))
	for _, monitor := range c.Monitors {
		trimmed := strings.TrimSpace(monitor)
		if trimmed == "" {
			continue
		}
		monitors = append(monitors, trimmed)
	}
	return strings.Join(monitors, ",")
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
	if len(cfg.Monitors) == 0 {
		cfg.Monitors = []string{defaultMonitorHost()}
	}
	if strings.TrimSpace(cfg.KeyFile) == "" {
		user, keyFile := defaultCephAuth()
		if strings.TrimSpace(cfg.User) == "" {
			cfg.User = user
		}
		cfg.KeyFile = keyFile
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
