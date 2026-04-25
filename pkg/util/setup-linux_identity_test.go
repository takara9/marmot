package util

import (
	"encoding/hex"
	"testing"

	"github.com/takara9/marmot/api"
)

func TestMachineIDForServer_NormalizesUUID(t *testing.T) {
	spec := api.Server{
		Id: "abcde",
		Metadata: &api.Metadata{
			Uuid: StringPtr("550e8400-e29b-41d4-a716-446655440000"),
		},
	}

	got := machineIDForServer(spec)
	want := "550e8400e29b41d4a716446655440000"
	if got != want {
		t.Fatalf("machineIDForServer() = %s, want %s", got, want)
	}
}

func TestMachineIDForServer_FallbackDeterministicHex(t *testing.T) {
	spec := api.Server{Id: "a123456"}

	got1 := machineIDForServer(spec)
	got2 := machineIDForServer(spec)

	if got1 != got2 {
		t.Fatalf("machine-id must be deterministic: %s != %s", got1, got2)
	}
	if len(got1) != 32 {
		t.Fatalf("machine-id length = %d, want 32", len(got1))
	}
	if _, err := hex.DecodeString(got1); err != nil {
		t.Fatalf("machine-id must be hex string: %v", err)
	}
}

func TestHostIDBytes_DeterministicAndDistinct(t *testing.T) {
	a := hostIDBytes("550e8400e29b41d4a716446655440000")
	b := hostIDBytes("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	c := hostIDBytes("550e8400e29b41d4a716446655440000")

	if len(a) != 4 {
		t.Fatalf("hostid length = %d, want 4", len(a))
	}
	if string(a) != string(c) {
		t.Fatalf("hostid must be deterministic: %v != %v", a, c)
	}
	if string(a) == string(b) {
		t.Fatalf("different machine-id should produce different hostid: %v == %v", a, b)
	}
}
