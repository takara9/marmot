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

	t.Run("returns false for name-only non-persistent request", func(t *testing.T) {
		disk := api.Volume{Metadata: api.Metadata{Name: "data"}}
		if shouldResolvePreCreatedStorageVolume(disk) {
			t.Fatal("shouldResolvePreCreatedStorageVolume() = true, want false")
		}
	})

	t.Run("returns false when persistent is explicitly false", func(t *testing.T) {
		persistent := false
		disk := api.Volume{
			Metadata: api.Metadata{Name: "data"},
			Spec:     api.VolSpec{Persistent: &persistent},
		}
		if shouldResolvePreCreatedStorageVolume(disk) {
			t.Fatal("shouldResolvePreCreatedStorageVolume() = true, want false")
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
