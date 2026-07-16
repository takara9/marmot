package ceph

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

func defaultMonitorHost() string {
	return strings.TrimSpace(os.Getenv("CEPH_IPADDR"))
}

type tempKeyring struct {
	path string
	once sync.Once
}

func (t *tempKeyring) Cleanup() error {
	if t == nil || t.path == "" {
		return nil
	}

	var err error
	t.once.Do(func() {
		err = os.Remove(t.path)
		if err != nil && os.IsNotExist(err) {
			err = nil
		}
	})
	return err
}

func defaultCephAuth() (string, string, *tempKeyring) {
	secret := strings.TrimSpace(os.Getenv("CEPH_POOL_KEY"))
	if secret == "" {
		return "", "", nil
	}

	user := parseKeyringUser(secret)
	f, err := os.CreateTemp("", "marmot-ceph-pool-*.keyring")
	if err != nil {
		return user, "", nil
	}
	keyFile := f.Name()
	if _, writeErr := f.WriteString(ensureTrailingNewline(secret)); writeErr != nil {
		_ = f.Close()
		_ = os.Remove(keyFile)
		return user, "", nil
	}
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(keyFile)
		return user, "", nil
	}
	if chmodErr := os.Chmod(keyFile, 0o600); chmodErr != nil {
		_ = os.Remove(keyFile)
		return user, "", nil
	}
	return user, keyFile, &tempKeyring{path: keyFile}
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
	cleanup     *tempKeyring
}

type Client struct {
	cfg    Config
	runner Runner
}

func DefaultConfig() Config {
	user, keyFile, cleanup := defaultCephAuth()
	return Config{
		Monitors: []string{defaultMonitorHost()},
		User:     user,
		KeyFile:  keyFile,
		PoolByClass: map[string]string{
			"hdd":  defaultPoolByClass["hdd"],
			"ssd":  defaultPoolByClass["ssd"],
			"nvme": defaultPoolByClass["nvme"],
		},
		cleanup: cleanup,
	}
}

func NewClient(cfg Config, runner Runner) (*Client, error) {
	normalized := normalizeConfig(cfg)
	if normalized.MonitorHosts() == "" {
		return nil, fmt.Errorf("ceph monitors are required")
	}
	if strings.TrimSpace(os.Getenv("CEPH_POOL_KEY")) != "" && strings.TrimSpace(normalized.KeyFile) == "" {
		return nil, fmt.Errorf("failed to create keyring file from CEPH_POOL_KEY")
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Client{cfg: normalized, runner: runner}, nil
}

func (c Config) Cleanup() error {
	return c.cleanup.Cleanup()
}

func (c *Client) Cleanup() error {
	if c == nil {
		return nil
	}
	return c.cfg.Cleanup()
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
		user, keyFile, cleanup := defaultCephAuth()
		if strings.TrimSpace(cfg.User) == "" {
			cfg.User = user
		}
		cfg.KeyFile = keyFile
		cfg.cleanup = cleanup
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
