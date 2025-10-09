package main

import (
	_ "embed"
	"main/cmd/mactl2/cmd"
	//"main/cmd"
	//"github.com/takara9/marmot/cmd/mactl2/cmd"
	//"marmot.io/client/cmd"
)

//go:embed version.txt
var version string

func main() {
	cmd.Execute()
}
