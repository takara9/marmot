package marmotd

import (
	"testing"

	"github.com/takara9/marmot/api"
)

func TestShouldResolvePreCreatedStorageVolume(t *testing.T) {
	t.Run("returns true when volume id is specified", func(t *testing.T) {
		disk := api.Volume{Metadata: api.Metadata{Id: "abcde"}}
		if !shouldResolvePreCreatedStorageVolume(disk) {
			t.Fatal("shouldResolvePreCreatedStorageVolume() = false, want true")
		}
	})

	t.Run("returns true when persistent is true", func(t *testing.T) {
		persistent := true
		disk := api.Volume{
			Metadata: api.Metadata{Name: "shared-data"},
			Spec:     api.VolSpec{Persistent: &persistent},
		}
		if !shouldResolvePreCreatedStorageVolume(disk) {
			t.Fatal("shouldResolvePreCreatedStorageVolume() = false, want true")
		}
	})

	t.Run("returns true for name-only request without provisioning hints", func(t *testing.T) {
		disk := api.Volume{Metadata: api.Metadata{Name: "data"}}
		if !shouldResolvePreCreatedStorageVolume(disk) {
			t.Fatal("shouldResolvePreCreatedStorageVolume() = false, want true")
		}
	})

	t.Run("returns false for name-only request with provisioning hints", func(t *testing.T) {
		size := 10
		typ := "qcow2"
		disk := api.Volume{
			Metadata: api.Metadata{Name: "data"},
			Spec: api.VolSpec{
				Size: &size,
				Type: &typ,
			},
		}
		if !shouldResolvePreCreatedStorageVolume(disk) {
			t.Fatal("shouldResolvePreCreatedStorageVolume() = false, want true")
		}
	})

	t.Run("returns true when name exists and persistent is explicitly false", func(t *testing.T) {
		persistent := false
		disk := api.Volume{
			Metadata: api.Metadata{Name: "data"},
			Spec:     api.VolSpec{Persistent: &persistent},
		}
		if !shouldResolvePreCreatedStorageVolume(disk) {
			t.Fatal("shouldResolvePreCreatedStorageVolume() = false, want true")
		}
	})
}

func TestServerScopedStorageName(t *testing.T) {
	t.Run("returns empty for empty name", func(t *testing.T) {
		if got := serverScopedStorageName("", "12345"); got != "" {
			t.Fatalf("serverScopedStorageName() = %q, want empty", got)
		}
	})

	t.Run("keeps original name when server id is empty", func(t *testing.T) {
		if got := serverScopedStorageName("data", ""); got != "data" {
			t.Fatalf("serverScopedStorageName() = %q, want %q", got, "data")
		}
	})

	t.Run("appends server id suffix", func(t *testing.T) {
		if got := serverScopedStorageName("data", "abcde"); got != "data-abcde" {
			t.Fatalf("serverScopedStorageName() = %q, want %q", got, "data-abcde")
		}
	})

	t.Run("does not double append suffix", func(t *testing.T) {
		if got := serverScopedStorageName("data-abcde", "abcde"); got != "data-abcde" {
			t.Fatalf("serverScopedStorageName() = %q, want %q", got, "data-abcde")
		}
	})

	t.Run("trims input before suffixing", func(t *testing.T) {
		if got := serverScopedStorageName(" data ", " abcde "); got != "data-abcde" {
			t.Fatalf("serverScopedStorageName() = %q, want %q", got, "data-abcde")
		}
	})
}

func TestNormalizeNewStorageVolumeRequest(t *testing.T) {
	t.Run("defaults type to qcow2", func(t *testing.T) {
		size := 10
		disk := api.Volume{Spec: api.VolSpec{Size: &size}}
		got, err := normalizeNewStorageVolumeRequest(disk, 0, true)
		if err != nil {
			t.Fatalf("normalizeNewStorageVolumeRequest() unexpected error: %v", err)
		}
		if got.Spec.Type == nil || *got.Spec.Type != "qcow2" {
			t.Fatalf("normalizeNewStorageVolumeRequest() type = %v, want qcow2", got.Spec.Type)
		}
	})

	t.Run("rejects missing size when required", func(t *testing.T) {
		disk := api.Volume{}
		if _, err := normalizeNewStorageVolumeRequest(disk, 2, true); err == nil {
			t.Fatal("normalizeNewStorageVolumeRequest() expected error, got nil")
		}
	})

	t.Run("rejects unsupported type", func(t *testing.T) {
		size := 10
		typ := "raw"
		disk := api.Volume{Spec: api.VolSpec{Size: &size, Type: &typ}}
		if _, err := normalizeNewStorageVolumeRequest(disk, 1, true); err == nil {
			t.Fatal("normalizeNewStorageVolumeRequest() expected error, got nil")
		}
	})

	t.Run("accepts lvm with positive size", func(t *testing.T) {
		size := 1
		typ := "lvm"
		disk := api.Volume{Spec: api.VolSpec{Size: &size, Type: &typ}}
		if _, err := normalizeNewStorageVolumeRequest(disk, 0, true); err != nil {
			t.Fatalf("normalizeNewStorageVolumeRequest() unexpected error: %v", err)
		}
	})
}
