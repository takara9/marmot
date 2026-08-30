package virt

import (
	"testing"
)

func TestParseOVSOfport(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   int
	}{
		{name: "healthy port", output: "5\n", want: 5},
		{name: "unassigned port", output: "-1\n", want: -1},
		{name: "empty output", output: "", want: -1},
		{name: "error text", output: "ovs-vsctl: no row \"vnet0\" in table Interface\n", want: -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOVSOfport(tc.output); got != tc.want {
				t.Fatalf("parseOVSOfport(%q) = %d, want %d", tc.output, got, tc.want)
			}
		})
	}
}
