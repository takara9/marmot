package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	cf "github.com/takara9/marmot/pkg/config"
)

type DefaultConfig struct {
	ApiServerUrl  string `yaml:"api_server"`
	EtcdServerUrl string `yaml:"etcd_server"`
}

func (m *Marmotd) ReadHvConfig() (cf.Hypervisors_yaml, DefaultConfig, error) {
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

func (m *Marmotd) SetHvConfig(hvs cf.Hypervisors_yaml, cnf DefaultConfig) error {

	// ハイパーバイザー
	for _, hv := range hvs.Hvs {
		fmt.Println(hv)
		err := m.dbc.SetHypervisor(hv)
		if err != nil {
			return err
		}
	}

	// OSイメージテンプレート
	for _, hd := range hvs.Imgs {
		err := m.dbc.SetImageTemplate(hd)
		if err != nil {
			return err
		}
	}

	// シーケンス番号のリセット
	for _, sq := range hvs.Seq {
		err := m.dbc.CreateSeq(sq.Key, sq.Start, sq.Step)
		if err != nil {
			return err
		}
	}

	return nil
}
