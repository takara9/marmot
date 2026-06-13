package marmotd

import "testing"

func TestResolveServerImageModuleFromOS(t *testing.T) {
	tests := []struct {
		name      string
		osName    string
		osVersion string
		wantKey   string
		wantErr   bool
	}{
		{name: "ubuntu 22.04", osName: "ubuntu", osVersion: "22.04", wantKey: "ubuntu22.04"},
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

func TestDeriveOSFromVariant(t *testing.T) {
	tests := []struct {
		variant     string
		wantName    string
		wantVersion string
	}{
		{variant: "ubuntu22.04", wantName: "ubuntu", wantVersion: "22.04"},
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
