package util

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"marmot.io/config"
	db "github.com/takara9/marmot/pkg/db"
)

type DefaultConfig struct {
	ApiServerUrl  string `yaml:"api_server"`
	EtcdServerUrl string `yaml:"etcd_server"`
}

func ReadHvConfig() (config.Hypervisors_yaml, DefaultConfig, error) {
	var hvs config.Hypervisors_yaml
	var cnf DefaultConfig

	err := config.ReadConfig(filepath.Join(os.Getenv("HOME"), ".config_marmot"), &cnf)
	if err != nil {
		return hvs, cnf, err
	}

	// パラメータの取得
	configFile := flag.String("config", "hypervisor-config.yaml", "Hypervisor config file")
	flag.Parse()
	err = config.ReadYAML(*configFile, &hvs)
	if err != nil {
		return hvs, cnf, err
	}

	return hvs, cnf, nil
}

func SetHvConfig(hvs config.Hypervisors_yaml, cnf DefaultConfig) error {

	// etcdへ接続
	d, err := db.NewDatabase(cnf.EtcdServerUrl)
	if err != nil {
		return err
	}

	// ハイパーバイザー
	for _, hv := range hvs.Hvs {
		fmt.Println(hv)
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
