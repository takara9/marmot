package main

import (
	"fmt"
	"os"

	"marmot.io/util"
)

func main() {

	// ホームディレクトリの.config_marmotから
	// APIサーバーとetcdサーバーのURLを取得
	hvs, cnf, err := util.ReadHvConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// データベースに登録
	err = util.SetHvConfig(hvs, cnf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
