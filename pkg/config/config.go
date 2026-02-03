package config

import (
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// YAML形式のコンフィグファイルを構造体に読み込む
func ReadYamlConfig(fn string, yamlConfig interface{}) error {
	file, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer file.Close()

	byteData, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(byteData, yamlConfig)
	if err != nil {
		return err
	}
	return nil
}

