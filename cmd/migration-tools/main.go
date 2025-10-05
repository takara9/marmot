package main

import (
	"fmt"
	"os"

	"github.com/takara9/marmot/pkg/db"
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
	//err = ut.SetHvConfig(hvs, cnf)
	//if err != nil {
	//	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	//	os.Exit(1)
	//}

	fmt.Println(hvs)
	fmt.Println(cnf)

	// etcdへ接続
	d, err := db.NewDatabase(cnf.EtcdServerUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(d)
	// ハイパーバイザー
	//for _, hv := range hvs.Hvs {
	//	fmt.Println(hv)
	//	err := d.SetHypervisor(hv)
	//	if err != nil {
	//		return err
	//	}
	//}

}
