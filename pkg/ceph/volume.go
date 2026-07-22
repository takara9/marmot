package ceph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type VolumeRequest struct {
	Pool         string
	Image        string
	SizeGB       int
	StorageClass string
}

type VolumeInfo struct {
	Pool             string
	Image            string
	SizeBytes        uint64
	ProviderVolumeID string
}

func (c *Client) authArgs() []string {
	return nil
}

func (c *Client) runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	fullArgs := append(c.authArgs(), args...)
	return c.runner.Run(ctx, name, fullArgs...)
}

func (r VolumeRequest) ProviderVolumeID() string {
	return fmt.Sprintf("%s/%s", strings.TrimSpace(r.Pool), strings.TrimSpace(r.Image))
}

func (c *Client) CreateVolume(ctx context.Context, req VolumeRequest) error {
	if strings.TrimSpace(req.Pool) == "" {
		return fmt.Errorf("pool is required")
	}
	if strings.TrimSpace(req.Image) == "" {
		return fmt.Errorf("image is required")
	}
	if req.SizeGB < 1 {
		return fmt.Errorf("size must be at least 1GB")
	}

	_, err := c.runCommand(ctx, "rbd", "create", req.ProviderVolumeID(), "--size", fmt.Sprintf("%dG", req.SizeGB))
	return err
}

func (c *Client) DeleteVolume(ctx context.Context, pool, image string) error {
	if strings.TrimSpace(pool) == "" {
		return fmt.Errorf("pool is required")
	}
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("image is required")
	}
	_, err := c.runCommand(ctx, "rbd", "rm", fmt.Sprintf("%s/%s", strings.TrimSpace(pool), strings.TrimSpace(image)))
	return err
}

func (c *Client) StatVolume(ctx context.Context, pool, image string) (VolumeInfo, error) {
	if strings.TrimSpace(pool) == "" {
		return VolumeInfo{}, fmt.Errorf("pool is required")
	}
	if strings.TrimSpace(image) == "" {
		return VolumeInfo{}, fmt.Errorf("image is required")
	}

	output, err := c.runCommand(ctx, "rbd", "info", fmt.Sprintf("%s/%s", strings.TrimSpace(pool), strings.TrimSpace(image)), "--format", "json")
	if err != nil {
		return VolumeInfo{}, err
	}

	var info struct {
		Name string `json:"name"`
		Size uint64 `json:"size"`
		Pool string `json:"pool"`
	}
	if err := json.Unmarshal(output, &info); err != nil {
		return VolumeInfo{}, fmt.Errorf("failed to decode rbd info output: %w", err)
	}
	if strings.TrimSpace(info.Name) == "" {
		info.Name = strings.TrimSpace(image)
	}
	if strings.TrimSpace(info.Pool) == "" {
		info.Pool = strings.TrimSpace(pool)
	}

	return VolumeInfo{
		Pool:             info.Pool,
		Image:            info.Name,
		SizeBytes:        info.Size,
		ProviderVolumeID: fmt.Sprintf("%s/%s", info.Pool, info.Name),
	}, nil
}

func (c *Client) ListVolumes(ctx context.Context, pool string) ([]string, error) {
	if strings.TrimSpace(pool) == "" {
		return nil, fmt.Errorf("pool is required")
	}

	output, err := c.runCommand(ctx, "rbd", "ls", strings.TrimSpace(pool), "--format", "json")
	if err != nil {
		return nil, err
	}

	var images []string
	if err := json.Unmarshal(output, &images); err == nil {
		return images, nil
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return []string{}, nil
	}
	return strings.Fields(trimmed), nil
}