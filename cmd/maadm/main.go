package main

import (
	_ "embed"

	"github.com/takara9/marmot/cmd/maadm/cmd"
)

//go:embed version.txt
var Version string

func main() {
	cmd.Execute()
}
