package ceph

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

type CommandError struct {
	Command string
	Output  string
	Err     error
}

func (r ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, CommandError{
			Command: strings.Join(append([]string{name}, args...), " "),
			Output:  strings.TrimSpace(string(output)),
			Err:     err,
		}
	}
	return output, nil
}

func (e CommandError) Error() string {
	if e.Output == "" {
		return fmt.Sprintf("command failed: %s: %v", e.Command, e.Err)
	}
	return fmt.Sprintf("command failed: %s: %v (output: %s)", e.Command, e.Err, e.Output)
}

func (e CommandError) Unwrap() error {
	return e.Err
}