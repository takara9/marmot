package cmd

import "testing"

func TestDeleteAliasResolvesToDelCommand(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"delete"})
	if err != nil {
		t.Fatalf("Find(delete) returned error: %v", err)
	}
	if cmd == nil {
		t.Fatalf("Find(delete) returned nil command")
	}
	if cmd.Name() != delCmd.Name() {
		t.Fatalf("Find(delete) resolved to %q, want %q", cmd.Name(), delCmd.Name())
	}

	aliasCmd, _, err := rootCmd.Find([]string{"del"})
	if err != nil {
		t.Fatalf("Find(del) returned error: %v", err)
	}
	if aliasCmd == nil {
		t.Fatalf("Find(del) returned nil command")
	}
	if aliasCmd.Name() != delCmd.Name() {
		t.Fatalf("Find(del) resolved to %q, want %q", aliasCmd.Name(), delCmd.Name())
	}
}
