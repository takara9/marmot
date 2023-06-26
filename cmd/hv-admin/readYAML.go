package main

import (
	"github.com/go-yaml/yaml"
	"os"
)

func readYAML(fn string, yf interface{}) error {
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
