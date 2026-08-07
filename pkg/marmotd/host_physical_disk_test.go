package marmotd

import (
	"errors"
	"testing"
)

func TestCollectPhysicalDiskCapacityUnquotedSize(t *testing.T) {
	orig := lsblkScsiCommandOutput
	defer func() { lsblkScsiCommandOutput = orig }()

	lsblkScsiCommandOutput = func() ([]byte, error) {
		return []byte(`{"blockdevices": [{"name":"sda","size":1000000000000},{"name":"sdb","size":1000000000000}]}`), nil
	}

	diskCount, diskCapacityGB := collectPhysicalDiskCapacity()
	if diskCount != 2 {
		t.Fatalf("diskCount=%d, want 2", diskCount)
	}
	if diskCapacityGB != 1862 {
		t.Fatalf("diskCapacityGB=%d, want 1862", diskCapacityGB)
	}
}

func TestCollectPhysicalDiskCapacityQuotedSize(t *testing.T) {
	orig := lsblkScsiCommandOutput
	defer func() { lsblkScsiCommandOutput = orig }()

	lsblkScsiCommandOutput = func() ([]byte, error) {
		return []byte(`{"blockdevices": [{"name":"sda","size":"1000000000000"}]}`), nil
	}

	diskCount, diskCapacityGB := collectPhysicalDiskCapacity()
	if diskCount != 1 {
		t.Fatalf("diskCount=%d, want 1", diskCount)
	}
	if diskCapacityGB != 931 {
		t.Fatalf("diskCapacityGB=%d, want 931", diskCapacityGB)
	}
}

func TestCollectPhysicalDiskCapacityCommandError(t *testing.T) {
	orig := lsblkScsiCommandOutput
	defer func() { lsblkScsiCommandOutput = orig }()

	lsblkScsiCommandOutput = func() ([]byte, error) {
		return nil, errors.New("lsblk not found")
	}

	diskCount, diskCapacityGB := collectPhysicalDiskCapacity()
	if diskCount != 0 || diskCapacityGB != 0 {
		t.Fatalf("expected zero values on command error, got diskCount=%d diskCapacityGB=%d", diskCount, diskCapacityGB)
	}
}
