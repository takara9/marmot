package marmotd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestCheckImageBackingStore(t *testing.T) {
	t.Run("all backing stores exist", func(t *testing.T) {
		tempDir := t.TempDir()
		qcow2Path := filepath.Join(tempDir, "image.qcow2")
		lvPath := filepath.Join(tempDir, "boot-volume")

		if err := os.WriteFile(qcow2Path, []byte("qcow2"), 0644); err != nil {
			t.Fatalf("failed to create qcow2 file: %v", err)
		}
		if err := os.WriteFile(lvPath, []byte("lv"), 0644); err != nil {
			t.Fatalf("failed to create lv file: %v", err)
		}

		image := api.Image{
			Spec: &api.ImageSpec{
				Qcow2Path: util.StringPtr(qcow2Path),
				LvPath:    util.StringPtr(lvPath),
			},
		}

		if err := CheckImageBackingStore(image); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("missing qcow2 file is detected", func(t *testing.T) {
		tempDir := t.TempDir()
		missingPath := filepath.Join(tempDir, "missing.qcow2")

		image := api.Image{
			Spec: &api.ImageSpec{
				Qcow2Path: util.StringPtr(missingPath),
			},
		}

		err := CheckImageBackingStore(image)
		if err == nil {
			t.Fatal("expected missing qcow2 error")
		}
		if !strings.Contains(err.Error(), missingPath) {
			t.Fatalf("expected error to include missing path, got %v", err)
		}
	})

	t.Run("logical volume path is derived when lvPath is empty", func(t *testing.T) {
		image := api.Image{
			Spec: &api.ImageSpec{
				VolumeGroup:   util.StringPtr("vg1"),
				LogicalVolume: util.StringPtr("boot-image"),
			},
		}

		err := CheckImageBackingStore(image)
		if err == nil {
			t.Fatal("expected missing logical volume error")
		}
		if !strings.Contains(err.Error(), "/dev/vg1/boot-image") {
			t.Fatalf("expected derived lv path in error, got %v", err)
		}
	})
}