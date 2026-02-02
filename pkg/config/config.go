package config

import (
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// YAML形式のコンフィグファイルを構造体に読み込む
func ReadYamlConfig(fn string, yamlConfig interface{}) error {
	file, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer file.Close()

	byteData, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(byteData, yamlConfig)
	if err != nil {
		return err
	}
	return nil
}

/*
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

	configJson := convConfYaml2Json(configYaml)
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

func convConfYaml2Json(c MarmotConfig) api.MarmotConfig {
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
		if spec.Storage != nil {
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
		}
		vmSpec = append(vmSpec, v)
	}
	a.VmSpec = &vmSpec
	return a
}
*/
