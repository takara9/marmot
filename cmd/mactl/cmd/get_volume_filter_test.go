package cmd

import (
	"testing"

	"github.com/takara9/marmot/api"
)

func TestFilterDataKindVolumes(t *testing.T) {
	data := "data"
	osKind := "os"
	blank := ""

	volumes := []api.Volume{
		{Metadata: api.Metadata{Name: "v-data-1"}, Spec: api.VolSpec{Kind: &data}},
		{Metadata: api.Metadata{Name: "v-os"}, Spec: api.VolSpec{Kind: &osKind}},
		{Metadata: api.Metadata{Name: "v-nil"}, Spec: api.VolSpec{}},
		{Metadata: api.Metadata{Name: "v-blank"}, Spec: api.VolSpec{Kind: &blank}},
		{Metadata: api.Metadata{Name: "v-data-2"}, Spec: api.VolSpec{Kind: &data}},
	}

	got := filterDataKindVolumes(volumes, false)
	if len(got) != 2 {
		t.Fatalf("filterDataKindVolumes() len=%d, want 2", len(got))
	}
	if got[0].Metadata.Name != "v-data-1" {
		t.Fatalf("filterDataKindVolumes()[0]=%q, want v-data-1", got[0].Metadata.Name)
	}
	if got[1].Metadata.Name != "v-data-2" {
		t.Fatalf("filterDataKindVolumes()[1]=%q, want v-data-2", got[1].Metadata.Name)
	}

	gotAll := filterDataKindVolumes(volumes, true)
	if len(gotAll) != len(volumes) {
		t.Fatalf("filterDataKindVolumes(..., true) len=%d, want %d", len(gotAll), len(volumes))
	}
}
