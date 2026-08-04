package marmotd

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestAggregateHostVolumeCapacityByNode(t *testing.T) {
	volumes := []api.Volume{
		{Metadata: api.Metadata{NodeName: util.StringPtr("hv1")}, Spec: api.VolSpec{Type: util.StringPtr("qcow2"), Size: util.IntPtrInt(16)}},
		{Metadata: api.Metadata{NodeName: util.StringPtr("hv1")}, Spec: api.VolSpec{Type: util.StringPtr("ceph"), Size: util.IntPtrInt(10)}},
		{Metadata: api.Metadata{NodeName: util.StringPtr("hv1")}, Spec: api.VolSpec{Type: util.StringPtr("lvm"), Size: nil}},
		{Metadata: api.Metadata{NodeName: util.StringPtr("hv1")}, Spec: api.VolSpec{Type: util.StringPtr("lvm"), Size: util.IntPtrInt(0)}},
		{Metadata: api.Metadata{NodeName: util.StringPtr("hv1")}, Spec: api.VolSpec{Type: util.StringPtr("ceph"), Size: util.IntPtrInt(-2)}},
		{Metadata: api.Metadata{NodeName: util.StringPtr("hv2")}, Spec: api.VolSpec{Type: util.StringPtr("qcow2"), Size: util.IntPtrInt(99)}},
		{Metadata: api.Metadata{NodeName: nil}, Spec: api.VolSpec{Type: util.StringPtr("ceph"), Size: util.IntPtrInt(50)}},
	}

	diskCount, diskCapacityGB := aggregateHostVolumeCapacityByNode(volumes, "hv1")
	if diskCount != 5 {
		t.Fatalf("diskCount=%d, want 5", diskCount)
	}
	if diskCapacityGB != 26 {
		t.Fatalf("diskCapacityGB=%d, want 26", diskCapacityGB)
	}
}

func TestAggregateHostVolumeCapacityByNodeTrimmedNodeName(t *testing.T) {
	volumes := []api.Volume{
		{Metadata: api.Metadata{NodeName: util.StringPtr(" hv1 ")}, Spec: api.VolSpec{Size: util.IntPtrInt(8)}},
		{Metadata: api.Metadata{NodeName: util.StringPtr("hv1")}, Spec: api.VolSpec{Size: util.IntPtrInt(4)}},
	}

	diskCount, diskCapacityGB := aggregateHostVolumeCapacityByNode(volumes, "  hv1")
	if diskCount != 2 {
		t.Fatalf("diskCount=%d, want 2", diskCount)
	}
	if diskCapacityGB != 12 {
		t.Fatalf("diskCapacityGB=%d, want 12", diskCapacityGB)
	}
}

func TestAggregateHostVolumeCapacityByNodeEmptyNodeName(t *testing.T) {
	volumes := []api.Volume{
		{Metadata: api.Metadata{NodeName: util.StringPtr("hv1")}, Spec: api.VolSpec{Size: util.IntPtrInt(10)}},
	}

	diskCount, diskCapacityGB := aggregateHostVolumeCapacityByNode(volumes, "   ")
	if diskCount != 0 {
		t.Fatalf("diskCount=%d, want 0", diskCount)
	}
	if diskCapacityGB != 0 {
		t.Fatalf("diskCapacityGB=%d, want 0", diskCapacityGB)
	}
}
