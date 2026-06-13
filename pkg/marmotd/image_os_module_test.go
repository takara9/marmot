package marmotd

import (
	"context"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestResolveImageOSModuleFromSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		osName    string
		osVersion string
		wantKey   string
		wantErr   bool
	}{
		{name: "default", osName: "", osVersion: "", wantKey: "ubuntu22.04"},
		{name: "ubuntu2204", osName: "ubuntu", osVersion: "22.04", wantKey: "ubuntu22.04"},
		{name: "ubuntu2404", osName: "ubuntu", osVersion: "24.04", wantKey: "ubuntu24.04"},
		{name: "ubuntuFallback", osName: "ubuntu", osVersion: "26.04", wantKey: "ubuntu"},
		{name: "alpine323", osName: "alpine", osVersion: "3.23", wantKey: "alpine3.23"},
		{name: "alpineUnsupported", osName: "alpine", osVersion: "3.22", wantErr: true},
		{name: "unknown", osName: "debian", osVersion: "12", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mod, err := resolveImageOSModuleFromSpec(tt.osName, tt.osVersion)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveImageOSModuleFromSpec() error = %v", err)
			}
			if got := mod.key(); got != tt.wantKey {
				t.Fatalf("mod.key() = %q, want %q", got, tt.wantKey)
			}
		})
	}
}

func TestApplyFollowerImageSpecByOS(t *testing.T) {
	t.Parallel()

	head := api.Image{}
	head.Spec.Kind = util.StringPtr("os")
	head.Spec.Type = util.StringPtr("qcow2")
	head.Spec.SourceUrl = util.StringPtr("https://example.invalid/image.qcow2")
	head.Spec.OsName = util.StringPtr("alpine")
	head.Spec.OsVersion = util.StringPtr("3.23")
	head.Spec.Size = util.IntPtrInt(20)

	follower := api.Image{}
	key, err := ApplyFollowerImageSpecByOS(&follower, head)
	if err != nil {
		t.Fatalf("ApplyFollowerImageSpecByOS() error = %v", err)
	}
	if key != "alpine3.23" {
		t.Fatalf("module key = %q, want %q", key, "alpine3.23")
	}
	if follower.Spec.Kind == nil || *follower.Spec.Kind != "os" {
		t.Fatalf("Kind = %#v", follower.Spec.Kind)
	}
	if follower.Spec.Type == nil || *follower.Spec.Type != "qcow2" {
		t.Fatalf("Type = %#v", follower.Spec.Type)
	}
	if follower.Spec.SourceUrl == nil || *follower.Spec.SourceUrl != "https://example.invalid/image.qcow2" {
		t.Fatalf("SourceUrl = %#v", follower.Spec.SourceUrl)
	}
	if follower.Spec.OsName == nil || *follower.Spec.OsName != "alpine" {
		t.Fatalf("OsName = %#v", follower.Spec.OsName)
	}
	if follower.Spec.OsVersion == nil || *follower.Spec.OsVersion != "3.23" {
		t.Fatalf("OsVersion = %#v", follower.Spec.OsVersion)
	}
	if follower.Spec.Size == nil || *follower.Spec.Size != 20 {
		t.Fatalf("Size = %#v", follower.Spec.Size)
	}
}

func TestImageModuleCustomizeDelegatesToCommonCustomizer(t *testing.T) {
	t.Parallel()

	// Empty path should fail in the shared customizer, proving delegation works.
	mod, err := resolveImageOSModuleFromSpec("ubuntu", "22.04")
	if err != nil {
		t.Fatalf("resolveImageOSModuleFromSpec() error = %v", err)
	}
	if err := mod.customizeDownloadedImage(context.Background(), ""); err == nil {
		t.Fatalf("expected customizeDownloadedImage to return error for empty path")
	}
}
