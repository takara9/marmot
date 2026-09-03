package db

import "testing"

func TestCompareKubernetesVersions(t *testing.T) {
	cases := []struct {
		name    string
		a, b    string
		want    int // -1, 0, 1 (符号のみ比較)
		wantErr bool
	}{
		{name: "equal minor", a: "1.30", b: "1.30", want: 0},
		{name: "equal patch", a: "1.30.2", b: "1.30.2", want: 0},
		{name: "upgrade minor", a: "1.31", b: "1.30", want: 1},
		{name: "downgrade minor", a: "1.29", b: "1.30", want: -1},
		{name: "upgrade major", a: "2.0", b: "1.36", want: 1},
		{name: "downgrade patch", a: "1.30.1", b: "1.30.2", want: -1},
		{name: "invalid format a", a: "1", b: "1.30", wantErr: true},
		{name: "invalid format b", a: "1.30", b: "abc", wantErr: true},
		{name: "invalid too many segments", a: "1.30.2.1", b: "1.30", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compareKubernetesVersions(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("compareKubernetesVersions(%q, %q) expected error, got nil", tc.a, tc.b)
				}
				return
			}
			if err != nil {
				t.Fatalf("compareKubernetesVersions(%q, %q) unexpected error: %v", tc.a, tc.b, err)
			}
			sign := 0
			if got > 0 {
				sign = 1
			} else if got < 0 {
				sign = -1
			}
			if sign != tc.want {
				t.Fatalf("compareKubernetesVersions(%q, %q) = %d, want sign %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
