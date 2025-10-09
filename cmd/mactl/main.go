package main

import (
	_ "embed"

	"github.com/takara9/marmot/cmd/mactl/cmd"
)

//go:embed version.txt
var version string

func main() {
	cmd.Execute()
}
