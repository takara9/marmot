package main

import (
	"fmt"
	"os"

	"github.com/takara9/marmot/pkg/config"
)

func main() {

	// ホームディレクトリの.config_marmotから
	// APIサーバーとetcdサーバーのURLを取得
	hvs, cnf, err := config.ReadHvConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// データベースに登録
	err = config.SetHvConfig(hvs, cnf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
