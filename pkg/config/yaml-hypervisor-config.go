package config

/*
	ハイパーバイザーのセットアップ用YAMLの型定義
*/

type StgPool_yaml struct {
	VolGroup string `yaml:"vg"`
	Type     string `yaml:"type"`
}

// ハイパーバイザー
type Hypervisor_yaml struct {
	Name    string         `yaml:"name"`
	IpAddr  string         `yaml:"ip_addr"`
	Port    uint64         `yaml:"port"`
	Cpu     uint64         `yaml:"cpu"`
	CpuFree uint64         `yaml:"free_cpu"`
	Ram     uint64         `yaml:"ram"`
	RamFree uint64         `yaml:"free_ram"`
	Storage []StgPool_yaml `yaml:"storage_pool"`
}

type Hypervisors_yaml struct {
	Hvs  []Hypervisor_yaml `yaml:"hv_spec"`
	Imgs []Image_yaml      `yaml:"image_template"`
	Seq  []SeqNo_yaml      `yaml:"seqno"`
}

// OSイメージ　テンプレート
type Image_yaml struct {
	Name          string `yaml:"name"`
	VolumeGroup   string `yaml:"volumegroup"`
	LogicalVolume string `yaml:"logicalvolume"`
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
