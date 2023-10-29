// YAMLを読み取るための型

package main

/*
type StgPool struct {
	VolGroup string `yaml:"vg"`
	Type     string `yaml:"type"`
}

// ハイパーバイザー
type Hypervisor struct {
	Name    string    `yaml:"name"`
	Cpu     uint64    `yaml:"cpu"`
	CpuFree uint64    `yaml:"free_cpu"`
	Ram     uint64    `yaml:"ram"`
	RamFree uint64    `yaml:"free_ram"`
	IpAddr  string    `yaml:"ip_addr"`
	Storage []StgPool `yaml:"storage_pool"`
}

type Hypervisors struct {
	Hvs  []Hypervisor `yaml:"hv_spec"`
	Imgs []Image      `yaml:"image_template"`
	Seq  []SeqNo      `yaml:"seqno"`
}

// OSイメージ　テンプレート
type Image struct {
	Name          string `yaml:"name"`
	VolumeGroup   string `yaml:"volumegroup"`
	LogicalVolume string `yaml:"logicalvolume"`
}

// シーケンス番号
type SeqNo struct {
	Start uint64 `yaml:"start"`
	Step  uint64 `yaml:"step"`
	Key   string `yaml:"name"`
}
*/