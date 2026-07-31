package cmd

import "testing"

func TestRootHasKubectlLikeServerLifecycleCommands(t *testing.T) {
	testCases := []struct {
		path       []string
		parentName string
	}{
		{path: []string{"start", "server"}, parentName: "start"},
		{path: []string{"stop", "server"}, parentName: "stop"},
		{path: []string{"restart", "server"}, parentName: "restart"},
	}

	for _, tc := range testCases {
		cmd, _, err := rootCmd.Find(tc.path)
		if err != nil {
			t.Fatalf("Find(%v) returned error: %v", tc.path, err)
		}
		if cmd == nil {
			t.Fatalf("Find(%v) returned nil command", tc.path)
		}
		if cmd.Name() != "server" {
			t.Fatalf("Find(%v) resolved to %q, want %q", tc.path, cmd.Name(), "server")
		}
		if cmd.Parent() == nil || cmd.Parent().Name() != tc.parentName {
			t.Fatalf("Find(%v) parent = %v, want %q", tc.path, cmd.Parent(), tc.parentName)
		}
	}
}
