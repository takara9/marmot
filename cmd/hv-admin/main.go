package main

import (
	"fmt"
	ut "github.com/takara9/marmot/pkg/util"
	"os"
)

func main() {

	hvs, cnf, err := ut.ReadHvConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	err = ut.SetHvConfig(hvs, cnf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
