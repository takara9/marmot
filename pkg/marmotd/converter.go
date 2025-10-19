package marmotd

import (
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/types"
)

// HVのデータベース保存形式を新API形式に変換する
func convHVinfoDBtoAPI(hv types.Hypervisor) api.Hypervisor {
	var memory int64 = int64(hv.Memory)
	var ipaddr string = hv.IpAddr
	var freecpu int32 = int32(hv.FreeCpu)
	var freememory int64 = int64(hv.FreeMemory)
	var status int32 = int32(hv.Status)
	var stgpool []api.StoragePool
	for _, v := range hv.StgPool {
		vg := v.VolGroup
		fc := int64(v.FreeCap)
		vc := int64(v.VgCap)
		tp := v.Type
		stgpool = append(stgpool, api.StoragePool{
			VolGroup: &vg,
			FreeCap:  &fc,
			VgCap:    &vc,
			Type:     &tp,
		})
	}
	return api.Hypervisor{
		NodeName:   hv.Nodename,
		IpAddr:     &ipaddr,
		Cpu:        int32(hv.Cpu),
		FreeCpu:    &freecpu,
		Memory:     &memory,
		FreeMemory: &freememory,
		Status:     &status,
		StgPool:    &stgpool,
	}
}

// VMのデータベース保存形式を新API形式に変換する
func convVMinfoDBtoAPI(vms []types.VirtualMachine) []api.VirtualMachine {
	var vms2 []api.VirtualMachine
	for _, vm := range vms {
		var memory int64 = int64(vm.Memory)
		var cpu int32 = int32(vm.Cpu)
		var status int32 = int32(vm.Status)
		var uuid string = vm.Uuid.String()
		var nodename string = vm.Name
		var privateIp string = vm.PrivateIp
		var publicIp string = vm.PublicIp
		var key string = vm.Key
		var hvnode string = vm.HvNode
		var clustername string = vm.ClusterName
		var comment string = vm.Comment
		var ctime time.Time = vm.Ctime
		var stime time.Time = vm.Stime
		var oslv string = vm.OsLv
		var osvariant string = vm.OsVariant
		var osvgs string = vm.OsVg
		var playbook string = vm.Playbook
		var storages []api.Storage

		for _, s := range vm.Storage {
			var size int64 = int64(s.Size)
			storages = append(storages, api.Storage{
				Name: &s.Name,
				Path: &s.Path,
				Size: &size,
				Vg:   &s.Vg,
				Lv:   &s.Lv,
			})
		}

		vms2 = append(vms2, api.VirtualMachine{
			Uuid:        &uuid,
			Name:        nodename,
			PrivateIp:   &privateIp,
			PublicIp:    &publicIp,
			Cpu:         &cpu,
			Memory:      &memory,
			Status:      &status,
			Key:         &key,
			HvNode:      hvnode,
			ClusterName: &clustername,
			Comment:     &comment,
			CTime:       &ctime,
			STime:       &stime,
			OsLv:        &oslv,
			OsVg:        &osvgs,
			OsVariant:   &osvariant,
			Playbook:    &playbook,
			Storage:     &storages,
		})
	}
	return vms2
}

// 新APIから旧APIの構造体へ変換する
func ConvConfClusterNew2Old(acnf api.MarmotConfig) config.MarmotConfig {
	var cnf config.MarmotConfig
	if acnf.ClusterName != nil {
		cnf.ClusterName = *acnf.ClusterName
	}
	if acnf.Domain != nil {
		cnf.Domain = *acnf.Domain
	}
	if acnf.Hypervisor != nil {
		cnf.Hypervisor = *acnf.Hypervisor
	}
	if acnf.ImageDefaultPath != nil {
		cnf.VMImageDfltPath = *acnf.ImageDefaultPath
	}
	if acnf.ImgaeTemplatePath != nil {
		cnf.VmImageTempPath = *acnf.ImgaeTemplatePath
	}
	if acnf.NetDevDefault != nil {
		cnf.NetDevDefault = *acnf.NetDevDefault
	}
	if acnf.NetDevPrivate != nil {
		cnf.NetDevPrivate = *acnf.NetDevPrivate
	}
	if acnf.OsVariant != nil {
		cnf.VMOsVariant = *acnf.OsVariant
	}
	if acnf.PrivateIpSubnet != nil {
		cnf.PrivateIPSubnet = *acnf.PrivateIpSubnet
	}
	if acnf.PublicIpDns != nil {
		cnf.PublicIPDns = *acnf.PublicIpDns
	}
	if acnf.PublicIpGw != nil {
		cnf.PublicIPGw = *acnf.PublicIpGw
	}
	if acnf.PublicIpSubnet != nil {
		cnf.PublicIPSubnet = *acnf.PublicIpSubnet
	}
	if acnf.Qcow2Image != nil {
		cnf.VMImageQCOW = *acnf.Qcow2Image
	}
	// ここでエラーになるのは、クライアントライブラリが未完のため
	for _, v := range *acnf.VmSpec {
		var vm config.VMSpec
		if v.Name != nil {
			vm.Name = *v.Name
		}
		if v.Cpu != nil {
			vm.CPU = int(*v.Cpu)
		}
		if v.Memory != nil {
			vm.Memory = int(*v.Memory)
		}
		if v.PrivateIp != nil {
			vm.PrivateIP = *v.PrivateIp
		}
		if v.PublicIp != nil {
			vm.PublicIP = *v.PublicIp
		}
		if v.Comment != nil {
			vm.Comment = *v.Comment
		}
		if v.Key != nil {
			vm.Key = *v.Key
		}
		if v.Ostemplv != nil {
			vm.OsTempLv = *v.Ostemplv
		}
		if v.Ostempvg != nil {
			vm.OsTempVg = *v.Ostempvg
		}
		if v.Ostempvariant != nil {
			vm.VMOsVariant = *v.Ostempvariant
		}
		if v.Uuid != nil {
			vm.Uuid = *v.Uuid
		}
		if v.Playbook != nil {
			vm.AnsiblePB = *v.Playbook
		}
		if v.Storage != nil {
			for _, v2 := range *v.Storage {
				var ss config.Storage
				if v2.Name != nil {
					ss.Name = *v2.Name
				}
				if v2.Path != nil {
					ss.Path = *v2.Path
				}
				if v2.Size != nil {
					ss.Size = int(*v2.Size)
				}
				if v2.Type != nil {
					ss.Type = *v2.Type
				}
				if v2.Vg != nil {
					ss.VolGrp = *v2.Vg
				}
				vm.Storage = append(vm.Storage, ss)
			}
		}
		cnf.VMSpec = append(cnf.VMSpec, vm)
	}
	return cnf
}

func ConvConfClusterOld2New(cnf config.MarmotConfig) api.MarmotConfig {
	var acnf api.MarmotConfig
	acnf.ClusterName = &cnf.ClusterName
	acnf.Domain = &cnf.Domain
	acnf.Hypervisor = &cnf.Hypervisor
	acnf.ImageDefaultPath = &cnf.VMImageDfltPath
	acnf.ImgaeTemplatePath = &cnf.VmImageTempPath
	acnf.Qcow2Image = &cnf.VMImageQCOW
	acnf.OsVariant = &cnf.VMOsVariant
	acnf.PrivateIpSubnet = &cnf.PrivateIPSubnet
	acnf.PublicIpDns = &cnf.PublicIPDns
	acnf.PublicIpGw = &cnf.PublicIPGw
	acnf.PublicIpSubnet = &cnf.PublicIPSubnet
	acnf.NetDevDefault = &cnf.NetDevDefault
	acnf.NetDevPrivate = &cnf.NetDevPrivate
	acnf.NetDevPublic = &cnf.NetDevPublic

	var vmSpec []api.VmSpec
	for _, spec := range cnf.VMSpec {
		var a api.VmSpec
		a.Name = &spec.Name

		m := int64(spec.Memory)
		a.Memory = &m

		c := int32(spec.CPU)
		a.Cpu = &c

		a.Comment = &spec.Comment
		a.Key = &spec.Key
		a.Ostemplv = &spec.OsTempLv
		a.Ostempvariant = &spec.VMOsVariant
		a.Ostempvg = &spec.OsTempVg
		a.Playbook = &spec.AnsiblePB
		a.PrivateIp = &spec.PrivateIP
		a.PublicIp = &spec.PublicIP
		a.Uuid = &spec.Uuid

		var storage []api.Storage
		for _, stg := range spec.Storage {
			var s api.Storage
			s.Name = &stg.Name
			size := int64(stg.Size)
			s.Size = &size
			s.Path = &stg.Path
			s.Vg = &stg.VolGrp
			s.Type = &stg.Type
			storage = append(storage, s)
		}
		a.Storage = &storage
		vmSpec = append(vmSpec, a)
	}
	acnf.VmSpec = &vmSpec
	return acnf
}
