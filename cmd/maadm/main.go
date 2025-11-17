package main

import (
	_ "embed"
	"log/slog"
	"os"

	"github.com/takara9/marmot/cmd/maadm/cmd"
)

func main() {
	// Setup slog
	opts := &slog.HandlerOptions{
		AddSource: true,
		//Level: slog.LevelDebug,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	slog.SetDefault(logger)
	cmd.Execute()
}
