package config

/*
	ハイパーバイザーのセットアップ用YAMLの型定義
*/

type StgPool_yaml struct {
	VolGroup string `yaml:"vg"`
	Type     string `yaml:"type"`
}

type Hypervisors_yaml struct {
	Imgs []Image_yaml `yaml:"image_template"`
	Seq  []SeqNo_yaml `yaml:"seqno"`
}

// OSイメージ　テンプレート
type Image_yaml struct {
	Name           string `yaml:"name"`
	VolumeGroup    string `yaml:"volumegroup"`
	LogicalVolume  string `yaml:"logicalvolume"`
	Qcow2ImagePath string `yaml:"qcow2_path"`
}

// シーケンス番号
type SeqNo_yaml struct {
	Start uint64 `yaml:"start"`
	Step  uint64 `yaml:"step"`
	Key   string `yaml:"name"`
}

type ClientConfig struct {
	ApiServerUrl string `yaml:"api_server"`
}
