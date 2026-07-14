package db

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestNormalizeVolumeSpecTypeAlias_TypeIscsiAlias(t *testing.T) {
	spec := api.VolSpec{Type: util.StringPtr("iscsi")}

	util.NormalizeVolumeSpecISCSIAlias(&spec)

	if spec.Type == nil || *spec.Type != "lvm" {
		t.Fatalf("type = %#v, want lvm", spec.Type)
	}
	if spec.Iscsi == nil || !*spec.Iscsi {
		t.Fatalf("iscsi = %#v, want true", spec.Iscsi)
	}
}

func TestNormalizeVolumeSpecTypeAlias_IscsiFlagWithoutType(t *testing.T) {
	spec := api.VolSpec{Iscsi: util.BoolPtr(true)}

	util.NormalizeVolumeSpecISCSIAlias(&spec)

	if spec.Type == nil || *spec.Type != "lvm" {
		t.Fatalf("type = %#v, want lvm", spec.Type)
	}
}
