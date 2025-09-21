package util

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	cf "github.com/takara9/marmot/pkg/config"
	db "github.com/takara9/marmot/pkg/db"
)

type DefaultConfig struct {
	ApiServerUrl  string `yaml:"api_server"`
	EtcdServerUrl string `yaml:"etcd_server"`
}

func ReadHvConfig() (cf.Hypervisors_yaml, DefaultConfig, error) {
	var hvs cf.Hypervisors_yaml
	var cnf DefaultConfig

	err := cf.ReadConfig(filepath.Join(os.Getenv("HOME"), ".config_marmot"), &cnf)
	if err != nil {
		return hvs, cnf, err
	}

	// パラメータの取得
	config := flag.String("config", "hypervisor-config.yaml", "Hypervisor config file")
	flag.Parse()
	err = cf.ReadYAML(*config, &hvs)
	if err != nil {
		return hvs, cnf, err
	}

	return hvs, cnf, nil
}

func SetHvConfig(hvs cf.Hypervisors_yaml, cnf DefaultConfig) error {

	// etcdへ接続
	d, err := db.NewDatabase(cnf.EtcdServerUrl)
	if err != nil {
		return err
	}

	// ハイパーバイザー
	for _, hv := range hvs.Hvs {
		fmt.Println(hv)
		err := d.SetHypervisor(hv)
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
