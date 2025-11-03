package config

import (
	"flag"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type DefaultConfig struct {
	ApiServerUrl  string `yaml:"api_server"`
	EtcdServerUrl string `yaml:"etcd_server"`
}

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

func ReadHypervisorConfig(yamlFileName string) (error, *Hypervisors_yaml) {
	var hvs Hypervisors_yaml

	if err := ReadConfig(yamlFileName, &hvs); err != nil {
		return err, nil
	}
	return nil, &hvs
}
