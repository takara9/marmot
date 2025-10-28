package config

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/takara9/marmot/pkg/db"
	"gopkg.in/yaml.v3"
)

func ReadYAML(fn string, yf interface{}) error {
	file, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(yf)
	if err != nil {
		return err
	}
	return nil
}

type DefaultConfig struct {
	ApiServerUrl  string `yaml:"api_server"`
	EtcdServerUrl string `yaml:"etcd_server"`
}

func ReadHvConfig() (Hypervisors_yaml, DefaultConfig, error) {
	var hvs Hypervisors_yaml
	var cnf DefaultConfig

	err := ReadConfig(filepath.Join(os.Getenv("HOME"), ".config_marmot"), &cnf)
	if err != nil {
		return hvs, cnf, err
	}

	// パラメータの取得
	config := flag.String("config", "hypervisor-config.yaml", "Hypervisor config file")
	flag.Parse()
	err = ReadYAML(*config, &hvs)
	if err != nil {
		return hvs, cnf, err
	}

	return hvs, cnf, nil
}

func SetHvConfig(hvs Hypervisors_yaml, cnf DefaultConfig) error {

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
