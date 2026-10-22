package marmotd

import (
	"testing"

	api "github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestDefaultOSVolumeSizeGB(t *testing.T) {
	tests := []struct {
		name string
		size *int
		want int
	}{
		{"unset", nil, 16},
		{"smaller than default", util.IntPtrInt(10), 16},
		{"larger than default", util.IntPtrInt(40), 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := api.Image{}
			img.Spec.Size = tt.size
			if got := defaultOSVolumeSizeGB(img); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRequestedOSVolumeSizeGBExplicitUndersized(t *testing.T) {
	img := api.Image{}
	img.Spec.Size = util.IntPtrInt(40)
	vol := api.Volume{}
	vol.Spec.Size = util.IntPtrInt(16)
	if _, err := requestedOSVolumeSizeGB(vol, img); err == nil {
		t.Fatal("expected error for undersized explicit request")
	}
	vol.Spec.Size = util.IntPtrInt(40)
	if got, err := requestedOSVolumeSizeGB(vol, img); err != nil || got != 40 {
		t.Fatalf("got %d, %v", got, err)
	}
}
