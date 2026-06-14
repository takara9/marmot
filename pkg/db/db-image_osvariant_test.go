package db

import "testing"

func TestDeriveImageOSFromVariant(t *testing.T) {
	tests := []struct {
		name       string
		variant    string
		wantName   string
		wantVer    string
	}{
		{
			name:     "ubuntu 22.04",
			variant:  "ubuntu22.04",
			wantName: "ubuntu",
			wantVer:  "22.04",
		},
		{
			name:     "ubuntu 24.04 with suffix",
			variant:  "ubuntu24.04-noble",
			wantName: "ubuntu",
			wantVer:  "24.04",
		},
		{
			name:     "alpine 3.23",
			variant:  "alpine3.23",
			wantName: "alpine",
			wantVer:  "3.23",
		},
		{
			name:    "unknown variant",
			variant: "custom-image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVer := deriveImageOSFromVariant(tt.variant)
			if gotName != tt.wantName || gotVer != tt.wantVer {
				t.Fatalf("deriveImageOSFromVariant(%q) = (%q, %q), want (%q, %q)", tt.variant, gotName, gotVer, tt.wantName, tt.wantVer)
			}
		})
	}
}
