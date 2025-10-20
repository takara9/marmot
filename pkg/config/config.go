package config

import (
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type Storage struct {
	Name   string `yaml:"name"`
	Size   int    `yaml:"size"`
	Path   string `yaml:"path"`
	VolGrp string `yaml:"vg"`
	Type   string `yaml:"type"` // ストレージの種類　hdd, ssd, nvme
}

type VMSpec struct {
	Name        string    `yaml:"name"` // OS hostname
	CPU         int       `yaml:"cpu"`
	Memory      int       `yaml:"memory"`
	PrivateIP   string    `yaml:"private_ip"`
	PublicIP    string    `yaml:"public_ip"`
	Storage     []Storage `yaml:"storage"`
	AnsiblePB   string    `yaml:"playbook"`
	Comment     string    `yaml:"comment"`
	Uuid        string    // VM UUID TX-Key
	Key         string    // VM NAME (etcd key)
	OsTempVg    string    // OS temp image VG
	OsTempLv    string    // OS temp image LV
	VMOsVariant string    // OS Variant
}

type MarmotConfig struct {
	Domain          string   `yaml:"domain"`
	ClusterName     string   `yaml:"cluster_name"`
	Hypervisor      string   `yaml:"hypervisor"`
	VmImageTempPath string   `yaml:"image_template_path"`
	VMImageDfltPath string   `yaml:"image_default_path"`
	VMImageQCOW     string   `yaml:"qcow2_image"`
	VMOsVariant     string   `yaml:"os_variant"`
	PrivateIPSubnet string   `yaml:"private_ip_subnet"`
	PublicIPSubnet  string   `yaml:"public_ip_subnet"`
	NetDevDefault   string   `yaml:"net_dev_default"`
	NetDevPrivate   string   `yaml:"net_dev_private"`
	NetDevPublic    string   `yaml:"net_dev_public"`
	PublicIPGw      string   `yaml:"public_ip_gw"`
	PublicIPDns     string   `yaml:"public_ip_dns"`
	VMSpec          []VMSpec `yaml:"vm_spec"`
}

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

func ReadConfig(fn string, yf interface{}) error {
	file, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer file.Close()

	byteData, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(byteData, yf)
	if err != nil {
		return err
	}
	return nil
}

func WriteConfig(fn string, yf interface{}) error {
	file, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer file.Close()

	byteData, err := yaml.Marshal(yf)
	if err != nil {
		return err
	}
	_, err = file.Write(byteData)
	if err != nil {
		return err
	}
	return nil
}
