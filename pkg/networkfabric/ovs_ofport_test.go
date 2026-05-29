package networkfabric

import "testing"

func TestParseInterfaceOfport_Ready(t *testing.T) {
	ofport, ready, err := parseInterfaceOfport("\"12\"")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Fatalf("expected ready=true")
	}
	if ofport != 12 {
		t.Fatalf("unexpected ofport: got %d, want 12", ofport)
	}
}

func TestParseInterfaceOfport_NotReadyMinusOne(t *testing.T) {
	ofport, ready, err := parseInterfaceOfport("-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatalf("expected ready=false")
	}
	if ofport != 0 {
		t.Fatalf("unexpected ofport: got %d, want 0", ofport)
	}
}

func TestParseInterfaceOfport_NotReadyZero(t *testing.T) {
	ofport, ready, err := parseInterfaceOfport("0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatalf("expected ready=false")
	}
	if ofport != 0 {
		t.Fatalf("unexpected ofport: got %d, want 0", ofport)
	}
}

func TestParseInterfaceOfport_InvalidText(t *testing.T) {
	_, _, err := parseInterfaceOfport("[]")
	if err == nil {
		t.Fatalf("expected error for invalid input")
	}
}

func TestParseInterfaceOfport_TrimmedQuotedValue(t *testing.T) {
	ofport, ready, err := parseInterfaceOfport("  \"7\"  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Fatalf("expected ready=true")
	}
	if ofport != 7 {
		t.Fatalf("unexpected ofport: got %d, want 7", ofport)
	}
}
