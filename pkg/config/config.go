package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type mactlClientConfig struct {
	ApiServerUrl string `yaml:"api_server"`
}

const maxYAMLConfigSize = 1024 * 1024

// YAML形式のコンフィグファイルを構造体に読み込む
func ReadYamlConfig(fn string, yamlConfig interface{}) error {
	byteData, err := readYAMLSource(fn)
	if err != nil {
		return err
	}

	if err := unmarshalYAMLUsingJSONTags(byteData, yamlConfig); err != nil {
		return err
	}
	return nil
}

func unmarshalYAMLUsingJSONTags(byteData []byte, out interface{}) error {
	var raw interface{}
	if err := yaml.Unmarshal(byteData, &raw); err != nil {
		return err
	}

	normalized := normalizeYAMLValue(raw)
	jsonData, err := json.Marshal(normalized)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(jsonData, out); err != nil {
		return err
	}
	return nil
}

func normalizeYAMLValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, child := range val {
			m[k] = normalizeYAMLValue(child)
		}
		return m
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, child := range val {
			m[fmt.Sprint(k)] = normalizeYAMLValue(child)
		}
		return m
	case []interface{}:
		for i := range val {
			val[i] = normalizeYAMLValue(val[i])
		}
		return val
	default:
		return v
	}
}

func readYAMLSource(source string) ([]byte, error) {
	if isHTTPURL(source) {
		return readYAMLFromURL(source)
	}
	return readYAMLFromFile(source)
}

func readYAMLFromFile(fn string) ([]byte, error) {
	file, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	byteData, err := io.ReadAll(io.LimitReader(file, maxYAMLConfigSize+1))
	if err != nil {
		return nil, err
	}
	if len(byteData) > maxYAMLConfigSize {
		return nil, fmt.Errorf("config file too large: %s", fn)
	}
	return byteData, nil
}

func readYAMLFromURL(source string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(source)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch yaml config from url %q: %w", source, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch yaml config from url %q: %s", source, resp.Status)
	}

	byteData, err := io.ReadAll(io.LimitReader(resp.Body, maxYAMLConfigSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read yaml config from url %q: %w", source, err)
	}
	if len(byteData) > maxYAMLConfigSize {
		return nil, fmt.Errorf("yaml config from url %q is too large", source)
	}
	return byteData, nil
}

func isHTTPURL(source string) bool {
	u, err := url.Parse(source)
	if err != nil {
		return false
	}

	scheme := strings.ToLower(u.Scheme)
	return (scheme == "http" || scheme == "https") && u.Host != ""
}
