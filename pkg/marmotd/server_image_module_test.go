package marmotd

import (
	"encoding/json"
	"testing"

	"github.com/takara9/marmot/api"
)

func TestResolveServerImageModuleFromOS(t *testing.T) {
	tests := []struct {
		name      string
		osName    string
		osVersion string
		wantKey   string
		wantErr   bool
	}{
		{name: "ubuntu 24.04", osName: "ubuntu", osVersion: "24.04", wantKey: "ubuntu24.04"},
		{name: "ubuntu 24.04", osName: "ubuntu", osVersion: "24.04", wantKey: "ubuntu24.04"},
		{name: "alpine 3.23", osName: "alpine", osVersion: "3.23", wantKey: "alpine3.23"},
		{name: "ubuntu fallback", osName: "ubuntu", osVersion: "26.04", wantKey: "ubuntu"},
		{name: "unsupported alpine", osName: "alpine", osVersion: "3.24", wantErr: true},
		{name: "unsupported os", osName: "debian", osVersion: "13", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod, err := resolveServerImageModuleFromOS(tt.osName, tt.osVersion)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mod == nil {
				t.Fatalf("module is nil")
			}
			if got := mod.Key(); got != tt.wantKey {
				t.Fatalf("module key = %q, want %q", got, tt.wantKey)
			}
		})
	}
}

func TestNormalizeServerImageDefault(t *testing.T) {
	mmImage := "alpine3.23"
	legacyServer := api.Server{}
	if err := json.Unmarshal([]byte(`{"spec":{"osVariant":"ubuntu22.04"}}`), &legacyServer); err != nil {
		t.Fatalf("failed to build legacy server input: %v", err)
	}
	tests := []struct {
		name      string
		server    api.Server
		wantImage string
	}{
		{name: "default", server: api.Server{}, wantImage: "ubuntu24.04"},
		{name: "mmImage", server: api.Server{Spec: api.ServerSpec{MmImage: &mmImage}}, wantImage: "alpine3.23"},
		{name: "legacy osVariant", server: legacyServer, wantImage: "ubuntu22.04"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalizeServerImageDefault(&tt.server)
			if tt.server.Spec.MmImage == nil || *tt.server.Spec.MmImage != tt.wantImage {
				t.Fatalf("MmImage = %v, want %q", tt.server.Spec.MmImage, tt.wantImage)
			}
		})
	}
}

func TestDeriveOSFromVariant(t *testing.T) {
	tests := []struct {
		variant     string
		wantName    string
		wantVersion string
	}{
		{variant: "ubuntu24.04", wantName: "ubuntu", wantVersion: "24.04"},
		{variant: "ubuntu24.04", wantName: "ubuntu", wantVersion: "24.04"},
		{variant: "alpine3.23", wantName: "alpine", wantVersion: "3.23"},
		{variant: "unknown", wantName: "", wantVersion: ""},
	}

	for _, tt := range tests {
		t.Run(tt.variant, func(t *testing.T) {
			gotName, gotVersion := deriveOSFromVariant(tt.variant)
			if gotName != tt.wantName || gotVersion != tt.wantVersion {
				t.Fatalf("deriveOSFromVariant(%q) = (%q, %q), want (%q, %q)", tt.variant, gotName, gotVersion, tt.wantName, tt.wantVersion)
			}
		})
	}
}
