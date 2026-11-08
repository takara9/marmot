package clusteradm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// marmotd.json から、maadm が参照する項目だけを読む
type nodeConfig struct {
	NodeName                 string `json:"node_name"`
	EtcdURL                  string `json:"etcd_url"`
	DefaultUnderlayInterface string `json:"default_underlay_interface"`
}

func readNodeConfig(path string) (*nodeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	var cfg nodeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return &cfg, nil
}

// updateJSONStrings は JSON オブジェクトの文字列項目だけを更新する。
// 他の項目の値とキーの並び順は保持し、存在しないキーは末尾へ追加する。
func updateJSONStrings(data []byte, updates map[string]string) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, fmt.Errorf("config is not a JSON object")
	}

	var keys []string
	values := map[string]json.RawMessage{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected token %v", tok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		if _, dup := values[key]; !dup {
			keys = append(keys, key)
		}
		values[key] = raw
	}

	for key, val := range updates {
		raw, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}
		if _, exists := values[key]; !exists {
			keys = append(keys, key)
		}
		values[key] = raw
	}

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, _ := json.Marshal(key)
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(values[key])
	}
	buf.WriteByte('}')

	var out bytes.Buffer
	if err := json.Indent(&out, buf.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// writeFileAtomic は同一ディレクトリの一時ファイルへ書いてから置き換える
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".maadm-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// fileBackup は変更前のファイルを *.bak へ退避し、復元できるようにする
type fileBackup struct {
	path    string
	existed bool
	mode    os.FileMode
}

func backupFile(path string) (*fileBackup, error) {
	b := &fileBackup{path: path, mode: 0o644}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return b, nil
	} else if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b.existed = true
	b.mode = info.Mode().Perm()
	if err := writeFileAtomic(path+".bak", data, b.mode); err != nil {
		return nil, fmt.Errorf("failed to write %s.bak: %w", path, err)
	}
	return b, nil
}

// restore は退避したファイルを戻す。元々存在しなかったファイルは削除する
func (b *fileBackup) restore() error {
	if !b.existed {
		if err := os.Remove(b.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := os.ReadFile(b.path + ".bak")
	if err != nil {
		return err
	}
	return writeFileAtomic(b.path, data, b.mode)
}
