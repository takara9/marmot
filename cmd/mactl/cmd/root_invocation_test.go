package cmd

import (
	"reflect"
	"testing"
)

func TestNormalizeInvocationArgs(t *testing.T) {
	testCases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "non mactl-ssh keeps args",
			in:   []string{"mactl", "get", "servers"},
			want: []string{"mactl", "get", "servers"},
		},
		{
			name: "mactl-ssh inserts ssh subcommand",
			in:   []string{"mactl-ssh", "ubuntu@web01", "--", "hostname"},
			want: []string{"mactl-ssh", "ssh", "ubuntu@web01", "--", "hostname"},
		},
		{
			name: "mactl-ssh without args still inserts ssh",
			in:   []string{"mactl-ssh"},
			want: []string{"mactl-ssh", "ssh"},
		},
		{
			name: "mactl-ssh with explicit ssh does not duplicate",
			in:   []string{"mactl-ssh", "ssh", "web01"},
			want: []string{"mactl-ssh", "ssh", "web01"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeInvocationArgs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("normalizeInvocationArgs() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
