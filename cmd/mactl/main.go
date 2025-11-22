package main

import (
	"log/slog"
	"os"

	"github.com/takara9/marmot/cmd/mactl/cmd"
)

// DEBUG Print
const DEBUG bool = true

func main() {

	// Setup slog
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	cmd.Execute()
}
