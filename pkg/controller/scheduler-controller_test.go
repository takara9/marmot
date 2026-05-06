package controller

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestClusterHasNode(t *testing.T) {
	statuses := []api.HostStatus{
		{NodeName: util.StringPtr("hvc")},
		{NodeName: util.StringPtr("ws1")},
		{},
	}

	tests := []struct {
		name     string
		nodeName string
		want     bool
	}{
		{name: "existing node", nodeName: "hvc", want: true},
		{name: "existing node with spaces", nodeName: " ws1 ", want: true},
		{name: "missing node", nodeName: "not-found", want: false},
		{name: "empty node", nodeName: "", want: false},
		{name: "spaces only", nodeName: "   ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clusterHasNode(statuses, tt.nodeName)
			if got != tt.want {
				t.Fatalf("clusterHasNode(%q) = %v, want %v", tt.nodeName, got, tt.want)
			}
		})
	}
}
