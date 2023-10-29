package config

import (
	"os"
	"gopkg.in/yaml.v3"
)

type StgPool_yaml struct {
	VolGroup string        `yaml:"vg"`
	Type     string        `yaml:"type"`
}

// ハイパーバイザー
type Hypervisor_yaml struct {
	Name    string         `yaml:"name"`
	Cpu     uint64         `yaml:"cpu"`
	CpuFree uint64         `yaml:"free_cpu"`
	Ram     uint64         `yaml:"ram"`
	RamFree uint64         `yaml:"free_ram"`
	IpAddr  string         `yaml:"ip_addr"`
	Storage []StgPool_yaml `yaml:"storage_pool"`
}

type Hypervisors_yaml struct {
	Hvs  []Hypervisor_yaml `yaml:"hv_spec"`
	Imgs []Image_yaml      `yaml:"image_template"`
	Seq  []SeqNo_yaml      `yaml:"seqno"`
}

// OSイメージ　テンプレート
type Image_yaml struct {
	Name          string   `yaml:"name"`
	VolumeGroup   string   `yaml:"volumegroup"`
	LogicalVolume string   `yaml:"logicalvolume"`
}

// シーケンス番号
type SeqNo_yaml struct {
	Start uint64 `yaml:"start"`
	Step  uint64 `yaml:"step"`
	Key   string `yaml:"name"`
}

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

/*
	Configでは、設定ファイルから、メモリへ読み込むまでを受け持つ
	DBへ書き出す行為は、ここでは扱わない
*/

// ハイパーバイザーの設定
/*
func SetHypervisor(v Hypervisor_yaml) (db.Hypervisor) {

	var hv db.Hypervisor
	hv.Nodename    = v.Name
	hv.Key         = v.Name             // Key
	hv.IpAddr      = v.IpAddr
	hv.Cpu         = int(v.Cpu)
	hv.FreeCpu     = int(v.Cpu)         // これで良いのか
	hv.Memory      = int(v.Ram * 1024)  // MB
	hv.FreeMemory  = int(v.Ram * 1024)
	hv.Status      = 0

	for _, val := range v.Storage {
		var sp StoragePool
		sp.VolGroup = val.VolGroup
		sp.Type     = val.Type
		hv.StgPool = append(hv.StgPool,sp)
	}
	return hv
}

// イメージテンプレート
func SetImageTemplate(con *etcd.Client,v Image_yaml) error {
	var osi OsImageTemplate
	osi.LogicaVol   = v.LogicalVolume
	osi.VolumeGroup = v.VolumeGroup
	osi.OsVariant   = v.Name
	key := fmt.Sprintf("%v_%v", "OSI", osi.OsVariant)
	err := PutDataEtcd(con, key, osi)
	if err != nil {
		log.Println("PutDataEtcd()", err)
		return err
	}
	return nil
}
*/