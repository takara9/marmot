package marmotd

import (
	"errors"
	"reflect"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestCleanupISCSIForVolumeRunsTargetAndBackstoreDelete(t *testing.T) {
	original := targetcliCommandOutput
	defer func() { targetcliCommandOutput = original }()

	var calls [][]string
	targetcliCommandOutput = func(args ...string) ([]byte, error) {
		calls = append(calls, append([]string{}, args...))
		return []byte("ok"), nil
	}

	m := &Marmot{}
	vol := api.Volume{
		Metadata: api.Metadata{Id: "abcde"},
		Spec: api.VolSpec{
			Type:           util.StringPtr("lvm"),
			Kind:           util.StringPtr("data"),
			Iscsi:          util.BoolPtr(true),
			IscsiTargetIqn: util.StringPtr("iqn.2024-01.com.marmot:target-abcde"),
		},
	}

	if err := m.cleanupISCSIForVolume(&vol); err != nil {
		t.Fatalf("cleanupISCSIForVolume() error = %v", err)
	}

	expected := [][]string{
		{"/iscsi", "delete", "iqn.2024-01.com.marmot:target-abcde"},
		{"/backstores/block", "delete", "disk-abcde"},
		{"saveconfig"},
	}
	if !reflect.DeepEqual(calls, expected) {
		t.Fatalf("unexpected targetcli calls\n got: %#v\nwant: %#v", calls, expected)
	}
}

func TestCleanupISCSIForVolumeSkipsNonISCSIVolume(t *testing.T) {
	original := targetcliCommandOutput
	defer func() { targetcliCommandOutput = original }()

	called := false
	targetcliCommandOutput = func(args ...string) ([]byte, error) {
		called = true
		return []byte("ok"), nil
	}

	m := &Marmot{}
	vol := api.Volume{
		Metadata: api.Metadata{Id: "abcde"},
		Spec: api.VolSpec{
			Type:  util.StringPtr("lvm"),
			Kind:  util.StringPtr("data"),
			Iscsi: util.BoolPtr(false),
		},
	}

	if err := m.cleanupISCSIForVolume(&vol); err != nil {
		t.Fatalf("cleanupISCSIForVolume() error = %v", err)
	}
	if called {
		t.Fatalf("targetcli should not be called for non-iscsi volume")
	}
}

func TestCleanupISCSIForVolumeAllowsMissingResources(t *testing.T) {
	original := targetcliCommandOutput
	defer func() { targetcliCommandOutput = original }()

	targetcliCommandOutput = func(args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "saveconfig" {
			return []byte("saved"), nil
		}
		return []byte("No such file or directory"), errors.New("not found")
	}

	m := &Marmot{}
	vol := api.Volume{
		Metadata: api.Metadata{Id: "abcde"},
		Spec: api.VolSpec{
			Type:  util.StringPtr("lvm"),
			Kind:  util.StringPtr("data"),
			Iscsi: util.BoolPtr(true),
		},
	}

	if err := m.cleanupISCSIForVolume(&vol); err != nil {
		t.Fatalf("cleanupISCSIForVolume() should ignore missing resources, but got error = %v", err)
	}
}

func TestCleanupISCSIForVolumeAllowsMissingBackstoreMessage(t *testing.T) {
	original := targetcliCommandOutput
	defer func() { targetcliCommandOutput = original }()

	targetcliCommandOutput = func(args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "saveconfig" {
			return []byte("saved"), nil
		}
		if len(args) >= 2 && args[0] == "/backstores/block" && args[1] == "delete" {
			return []byte("No storage object named disk-abcde."), errors.New("not found")
		}
		return []byte("ok"), nil
	}

	m := &Marmot{}
	vol := api.Volume{
		Metadata: api.Metadata{Id: "abcde"},
		Spec: api.VolSpec{
			Type:  util.StringPtr("lvm"),
			Kind:  util.StringPtr("data"),
			Iscsi: util.BoolPtr(true),
		},
	}

	if err := m.cleanupISCSIForVolume(&vol); err != nil {
		t.Fatalf("cleanupISCSIForVolume() should ignore missing backstore message, but got error = %v", err)
	}
}
