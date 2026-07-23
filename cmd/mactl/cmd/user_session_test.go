package cmd

import (
	"testing"

	"github.com/takara9/marmot/api"
)

func TestShortSessionID(t *testing.T) {
	got := shortSessionID("12345678-aaaa-bbbb-cccc-1234567890ab")
	if got != "12345678" {
		t.Fatalf("shortSessionID() = %q, want %q", got, "12345678")
	}

	got = shortSessionID("abcd")
	if got != "abcd" {
		t.Fatalf("shortSessionID() = %q, want %q", got, "abcd")
	}
}

func TestSessionFromIPText(t *testing.T) {
	if got := sessionFromIPText(api.ApiKey{}); got != "-" {
		t.Fatalf("sessionFromIPText() = %q, want %q", got, "-")
	}

	ip := "2001:db8::1"
	k := api.ApiKey{Spec: api.ApiKeySpec{FromIP: &ip}}
	if got := sessionFromIPText(k); got != ip {
		t.Fatalf("sessionFromIPText() = %q, want %q", got, ip)
	}
}
