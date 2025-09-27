/*
Copyright © 2025 Maho Takara <tkr9955@gmail.com>
*/
package main

import (
	_ "embed"

	"github.com/takara9/marmot/cmd/mactl2/cmd"
)

//go:embed version.txt
var version string

func main() {
	cmd.Execute()
}
