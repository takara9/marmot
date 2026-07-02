package marmotd

import "testing"

func TestShouldTreatIPAllocationAsReusable(t *testing.T) {
	tests := []struct {
		name          string
		ownerName     string
		requestServer string
		want          bool
	}{
		{name: "empty owner is reusable", ownerName: "", requestServer: "test-server-07", want: true},
		{name: "same owner is reusable", ownerName: "test-server-08", requestServer: "test-server-08", want: true},
		{name: "different owner is not reusable", ownerName: "test-server-08", requestServer: "test-server-07", want: false},
		{name: "trimmed same owner is reusable", ownerName: " test-server-08 ", requestServer: "test-server-08", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldTreatIPAllocationAsReusable(tc.ownerName, tc.requestServer)
			if got != tc.want {
				t.Fatalf("shouldTreatIPAllocationAsReusable(%q, %q) = %v, want %v", tc.ownerName, tc.requestServer, got, tc.want)
			}
		})
	}
}
