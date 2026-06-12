package marmotd

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

func TestSelectReusableVolume(t *testing.T) {
	t.Run("returns nil when no candidates exist", func(t *testing.T) {
		volume, err := selectReusableVolume(nil)
		if err != nil {
			t.Fatalf("selectReusableVolume() error = %v", err)
		}
		if volume != nil {
			t.Fatalf("selectReusableVolume() volume = %#v, want nil", volume)
		}
	})

	t.Run("returns the only candidate", func(t *testing.T) {
		candidate := api.Volume{
			Metadata: api.Metadata{Name: "volume1"},
			Status:   &api.Status{StatusCode: db.VOLUME_AVAILABLE},
		}

		volume, err := selectReusableVolume([]api.Volume{candidate})
		if err != nil {
			t.Fatalf("selectReusableVolume() error = %v", err)
		}
		if volume == nil {
			t.Fatal("selectReusableVolume() volume = nil, want candidate")
		}
		if volume.Metadata.Name != "volume1" {
			t.Fatalf("selectReusableVolume() name = %q, want volume1", volume.Metadata.Name)
		}
		if volume.Status == nil || volume.Status.StatusCode != db.VOLUME_AVAILABLE {
			t.Fatalf("selectReusableVolume() status = %#v, want available", volume.Status)
		}
	})

	t.Run("rejects duplicate candidates", func(t *testing.T) {
		candidate := api.Volume{
			Metadata: api.Metadata{Name: "volume1"},
			Status:   &api.Status{StatusCode: db.VOLUME_AVAILABLE},
		}

		volume, err := selectReusableVolume([]api.Volume{candidate, candidate})
		if err == nil {
			t.Fatal("selectReusableVolume() error = nil, want duplicate error")
		}
		if volume != nil {
			t.Fatalf("selectReusableVolume() volume = %#v, want nil", volume)
		}
	})
}