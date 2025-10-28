package main

import (
	"fmt"
	"os"

	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
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
	err = SetHvConfig(hvs, cnf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func SetHvConfig(hvs config.Hypervisors_yaml, cnf config.DefaultConfig) error {

	// etcdへ接続
	d, err := db.NewDatabase(cnf.EtcdServerUrl)
	if err != nil {
		return err
	}

	// ハイパーバイザー
	for _, hv := range hvs.Hvs {
		err := d.SetHypervisors(hv)
		if err != nil {
			return err
		}
	}

	// OSイメージテンプレート
	for _, hd := range hvs.Imgs {
		err := d.SetImageTemplate(hd)
		if err != nil {
			return err
		}
	}

	// シーケンス番号のリセット
	for _, sq := range hvs.Seq {
		err := d.CreateSeq(sq.Key, sq.Start, sq.Step)
		if err != nil {
			return err
		}
	}

	return nil
}
