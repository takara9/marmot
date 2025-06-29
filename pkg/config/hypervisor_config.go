package config

import (
	"gopkg.in/yaml.v3"
	"os"
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
