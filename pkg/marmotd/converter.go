package marmotd

import (
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/types"
)

// HVのデータベース保存形式を新API形式に変換する
func convHVinfoDBtoAPI(hv types.Hypervisor) api.Hypervisor {
	var memory int64 = int64(hv.Memory)
	var ipaddr string = hv.IpAddr
	var port int32 = int32(hv.Port)
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
		Port:				&port,
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
		var hvip string = vm.HvIpAddr
		var hvport int32 = int32(vm.HvPort)
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
			HvIpAddr:		 hvip,
			HvPort:      hvport,
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
