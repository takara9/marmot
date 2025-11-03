package main

import "github.com/takara9/marmot/cmd/maadm/cmd"


//go:embed version.txt
var version string


func main() {
	cmd.Execute()
}
