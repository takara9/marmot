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

func TestPrintLifecycleResultTextUsesResponseID(t *testing.T) {
	withOutputStyle("text", t, func() {
		out := captureStdoutForIDTest(t, func() {
			if err := printLifecycleResult([]byte(`{"id":"srv-123","metadata":{"id":"meta-999"}}`), "MSG"); err != nil {
				t.Fatalf("printLifecycleResult() unexpected err: %v", err)
			}
		})

		if !contains(out, "MSG meta-999") {
			t.Fatalf("stdout = %q, want contains %q", out, "MSG meta-999")
		}
	})
}

func TestServerLifecycleNoopStatusHelpers(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantStart  bool
		wantStop   bool
	}{
		{name: "running", statusCode: 2, wantStart: true, wantStop: false},
		{name: "starting", statusCode: 7, wantStart: true, wantStop: false},
		{name: "stopped", statusCode: 3, wantStart: false, wantStop: true},
		{name: "stopping", statusCode: 6, wantStart: false, wantStop: true},
		{name: "other", statusCode: 1, wantStart: false, wantStop: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := isStartNoopStatus(tc.statusCode); got != tc.wantStart {
				t.Fatalf("isStartNoopStatus(%d) = %v, want %v", tc.statusCode, got, tc.wantStart)
			}
			if got := isStopNoopStatus(tc.statusCode); got != tc.wantStop {
				t.Fatalf("isStopNoopStatus(%d) = %v, want %v", tc.statusCode, got, tc.wantStop)
			}
		})
	}
}

func TestPrintLifecycleResultFromIDUsesProvidedID(t *testing.T) {
	withOutputStyle("text", t, func() {
		out := captureStdoutForIDTest(t, func() {
			if err := printLifecycleResultFromID("srv-456", "MSG"); err != nil {
				t.Fatalf("printLifecycleResultFromID() unexpected err: %v", err)
			}
		})

		if !contains(out, "MSG srv-456") {
			t.Fatalf("stdout = %q, want contains %q", out, "MSG srv-456")
		}
	})
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
