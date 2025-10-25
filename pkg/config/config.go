package config

import (
	"io"
	"os"

	"github.com/takara9/marmot/api"
	"gopkg.in/yaml.v3"
)

/*
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
*/

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

func ReadYamlClusterConfig(configYamlFile string) (*api.MarmotConfig, error) {
	var configYaml MarmotConfig
	fd, err := os.Open(configYamlFile)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	byteData, err := io.ReadAll(fd)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(byteData, &configYaml)
	if err != nil {
		return nil, err
	}

	configJson := ConvConfYaml2Json(configYaml)

	return &configJson, nil
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

func ConvConfYaml2Json(c MarmotConfig) api.MarmotConfig {
	var a api.MarmotConfig

	a.ClusterName = c.ClusterName
	a.Domain = c.Domain
	a.Hypervisor = c.Hypervisor
	a.ImageDefaultPath = c.ImageDefaultPath
	a.ImgaeTemplatePath = c.ImgaeTemplatePath
	a.Qcow2Image = c.Qcow2Image
	a.OsVariant = c.OsVariant
	a.PrivateIpSubnet = c.PrivateIpSubnet
	a.PublicIpDns = c.PublicIpDns
	a.PublicIpGw = c.PublicIpGw
	a.PublicIpSubnet = c.PublicIpSubnet
	a.NetDevDefault = c.NetDevDefault
	a.NetDevPrivate = c.NetDevPrivate
	a.NetDevPublic = c.NetDevPublic

	var vmSpec []api.VmSpec
	for _, spec := range *c.VmSpec {
		var v api.VmSpec
		v.Name = spec.Name
		m := int64(*spec.Memory)
		v.Memory = &m
		c := int32(*spec.Cpu)
		v.Cpu = &c
		v.Comment = spec.Comment
		v.Key = spec.Key
		v.Ostemplv = spec.Ostemplv
		v.Ostempvariant = spec.Ostempvariant
		v.Ostempvg = spec.Ostempvg
		v.Playbook = spec.Playbook
		v.PrivateIp = spec.PrivateIp
		v.PublicIp = spec.PublicIp
		v.Uuid = spec.Uuid

		var storage []api.Storage
		for _, stg := range *spec.Storage {
			var s api.Storage
			s.Name = stg.Name
			size := int64(*stg.Size)
			s.Size = &size
			s.Path = stg.Path
			s.Vg = stg.Vg
			s.Type = stg.Type
			storage = append(storage, s)
		}
		v.Storage = &storage
		vmSpec = append(vmSpec, v)
	}
	a.VmSpec = &vmSpec
	return a
}

func ConvConfJson2Yaml(a api.MarmotConfig) MarmotConfig {
	var cnf MarmotConfig
	if a.ClusterName != nil {
		cnf.ClusterName = a.ClusterName
	}
	if a.Domain != nil {
		cnf.Domain = a.Domain
	}
	if a.Hypervisor != nil {
		cnf.Hypervisor = a.Hypervisor
	}
	if a.ImageDefaultPath != nil {
		cnf.ImageDefaultPath = a.ImageDefaultPath
	}
	if a.ImgaeTemplatePath != nil {
		cnf.ImgaeTemplatePath = a.ImgaeTemplatePath
	}
	if a.NetDevDefault != nil {
		cnf.NetDevDefault = a.NetDevDefault
	}
	if a.NetDevPrivate != nil {
		cnf.NetDevPrivate = a.NetDevPrivate
	}
	if a.OsVariant != nil {
		cnf.OsVariant = a.OsVariant
	}
	if a.PrivateIpSubnet != nil {
		cnf.PrivateIpSubnet = a.PrivateIpSubnet
	}
	if a.PublicIpDns != nil {
		cnf.PublicIpDns = a.PublicIpDns
	}
	if a.PublicIpGw != nil {
		cnf.PublicIpGw = a.PublicIpGw
	}
	if a.PublicIpSubnet != nil {
		cnf.PublicIpSubnet = a.PublicIpSubnet
	}
	if a.Qcow2Image != nil {
		cnf.Qcow2Image = a.Qcow2Image
	}
	for _, v := range *a.VmSpec {
		var vm VmSpec
		if v.Name != nil {
			vm.Name = v.Name
		}
		if v.Cpu != nil {
			vm.Cpu = v.Cpu
		}
		if v.Memory != nil {
			vm.Memory = v.Memory
		}
		if v.PrivateIp != nil {
			vm.PrivateIp = v.PrivateIp
		}
		if v.PublicIp != nil {
			vm.PublicIp = v.PublicIp
		}
		if v.Comment != nil {
			vm.Comment = v.Comment
		}
		if v.Key != nil {
			vm.Key = v.Key
		}
		if v.Ostemplv != nil {
			vm.Ostemplv = v.Ostemplv
		}
		if v.Ostempvg != nil {
			vm.Ostempvg = v.Ostempvg
		}
		if v.Ostempvariant != nil {
			vm.Ostempvariant = v.Ostempvariant
		}
		if v.Uuid != nil {
			vm.Uuid = v.Uuid
		}
		if v.Playbook != nil {
			vm.Playbook = v.Playbook
		}
		if v.Storage != nil {
			for _, vs := range *v.Storage {
				var ss Storage
				if vs.Name != nil {
					ss.Name = vs.Name
				}
				if vs.Path != nil {
					ss.Path = vs.Path
				}
				if vs.Size != nil {
					ss.Size = vs.Size
				}
				if vs.Type != nil {
					ss.Type = vs.Type
				}
				if vs.Vg != nil {
					ss.Vg = vs.Vg
				}
				*vm.Storage = append(*vm.Storage, ss)
			}
		}
		*cnf.VmSpec = append(*cnf.VmSpec, vm)
	}
	return cnf
}
