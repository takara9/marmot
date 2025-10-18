package main

import (
	"fmt"
	"os"

	ut "github.com/takara9/marmot/pkg/util"
)

func main() {

	// ホームディレクトリの.config_marmotから
	// APIサーバーとetcdサーバーのURLを取得
	hvs, cnf, err := ut.ReadHvConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// データベースに登録
	err = ut.SetHvConfig(hvs, cnf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
