package marmotd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestCheckVolumeBackingStore(t *testing.T) {
	t.Run("existing qcow2 file passes", func(t *testing.T) {
		tempDir := t.TempDir()
		qcow2Path := filepath.Join(tempDir, "disk.qcow2")

		if err := os.WriteFile(qcow2Path, []byte("qcow2"), 0644); err != nil {
			t.Fatalf("failed to create qcow2 file: %v", err)
		}

		volume := api.Volume{
			Spec: &api.VolSpec{
				Type: util.StringPtr("qcow2"),
				Path: util.StringPtr(qcow2Path),
			},
		}

		if err := CheckVolumeBackingStore(volume); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("missing qcow2 file is detected", func(t *testing.T) {
		missingPath := filepath.Join(t.TempDir(), "missing.qcow2")

		volume := api.Volume{
			Spec: &api.VolSpec{
				Type: util.StringPtr("qcow2"),
				Path: util.StringPtr(missingPath),
			},
		}

		err := CheckVolumeBackingStore(volume)
		if err == nil {
			t.Fatal("expected missing qcow2 error")
		}
		if !strings.Contains(err.Error(), missingPath) {
			t.Fatalf("expected error to include missing path, got %v", err)
		}
	})

	t.Run("logical volume path is derived when path is empty", func(t *testing.T) {
		volume := api.Volume{
			Spec: &api.VolSpec{
				Type:          util.StringPtr("lvm"),
				VolumeGroup:   util.StringPtr("vg1"),
				LogicalVolume: util.StringPtr("oslv-demo"),
			},
		}

		err := CheckVolumeBackingStore(volume)
		if err == nil {
			t.Fatal("expected missing logical volume error")
		}
		if !strings.Contains(err.Error(), "/dev/vg1/oslv-demo") {
			t.Fatalf("expected derived lv path in error, got %v", err)
		}
	})
}