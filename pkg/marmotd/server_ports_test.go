package marmotd

import (
	"strings"
	"testing"
)

func TestNormalizeServerPorts(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr string
	}{
		{
			name:    "empty list",
			in:      []string{},
			wantErr: "spec.serverPorts is required",
		},
		{
			name:    "contains empty entry",
			in:      []string{"http", "  "},
			wantErr: "spec.serverPorts contains an empty entry",
		},
		{
			name: "service name and explicit ports",
			in:   []string{"http", "1234/tcp", "5353/UDP"},
			want: []string{"80/tcp", "1234/tcp", "5353/udp"},
		},
		{
			name: "dedupe while preserving first occurrence order",
			in:   []string{"https", "443/tcp", "https", "443/tcp"},
			want: []string{"443/tcp"},
		},
		{
			name:    "numeric port without protocol is rejected",
			in:      []string{"1234"},
			wantErr: "must include protocol suffix",
		},
		{
			name:    "invalid protocol rejected",
			in:      []string{"53/sctp"},
			wantErr: "must be tcp or udp",
		},
		{
			name:    "unknown service rejected",
			in:      []string{"service-that-should-not-exist-xyz"},
			wantErr: "failed to resolve service name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeServerPorts(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeServerPorts() returned error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("NormalizeServerPorts() length = %d, want %d; got=%v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("NormalizeServerPorts()[%d] = %q, want %q; got=%v", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}
