package marmotd

import (
	"fmt"
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
		Port:       &port,
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
			HvIpAddr:    &hvip,
			HvPort:      &hvport,
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

// HVへVMスケジュールするために db.VirtualMachineにセットする
func convApiConfigToDB(spec api.VmSpec, cnf api.MarmotConfig) types.VirtualMachine {
	var vm types.VirtualMachine
	if cnf.ClusterName != nil {
		vm.ClusterName = *cnf.ClusterName
	}
	if cnf.OsVariant != nil {
		vm.OsVariant = *cnf.OsVariant
	}
	if spec.Name != nil {
		vm.Name = *spec.Name // Os のhostname
	}
	if spec.Cpu != nil {
		vm.Cpu = int(*spec.Cpu)
	}
	if spec.Memory != nil {
		vm.Memory = int(*spec.Memory)
	}
	if spec.PrivateIp != nil {
		vm.PrivateIp = *spec.PrivateIp
	}
	if spec.PublicIp != nil {
		vm.PublicIp = *spec.PublicIp
	}
	if spec.Playbook != nil {
		vm.Playbook = *spec.Playbook
	}
	if spec.Comment != nil {
		vm.Comment = *spec.Comment
	}
	vm.Status = types.INITALIZING
	if spec.Storage != nil {
		for _, stg := range *spec.Storage {
			var vms types.Storage
			vms.Name = *stg.Name
			vms.Size = int(*stg.Size)
			vms.Path = *stg.Path
			vm.Storage = append(vm.Storage, vms)
		}
	}
	return vm
}

// 新APIから旧APIの構造体へ変換する
func PrintMarmotConfig(a api.MarmotConfig) {
	fmt.Println("=========================================")
	if a.ClusterName != nil {
		fmt.Println("a.ClusterName=", *a.ClusterName)
	}
	if a.Domain != nil {
		fmt.Println("a.Domain=", *a.Domain)
	}
	if a.Hypervisor != nil {
		fmt.Println("a.Hypervisor=", *a.Hypervisor)
	}
	if a.ImageDefaultPath != nil {
		fmt.Println("a.ImageDefaultPath=", *a.ImageDefaultPath)
	}
	if a.ImgaeTemplatePath != nil {
		fmt.Println("a.ImgaeTemplatePath=", *a.ImgaeTemplatePath)
	}
	if a.NetDevDefault != nil {
		fmt.Println("a.NetDevDefault=", *a.NetDevDefault)
	}
	if a.NetDevPrivate != nil {
		fmt.Println("a.NetDevPrivate=", *a.NetDevPrivate)
	}
	if a.OsVariant != nil {
		fmt.Println("a.OsVariant=", *a.OsVariant)
	}
	if a.PrivateIpSubnet != nil {
		fmt.Println("a.PrivateIpSubnet=", *a.PrivateIpSubnet)
	}
	if a.PublicIpDns != nil {
		fmt.Println("a.PublicIpDns=", *a.PublicIpDns)
	}
	if a.PublicIpGw != nil {
		fmt.Println("a.PublicIpGw=", *a.PublicIpGw)
	}
	if a.PublicIpSubnet != nil {
		fmt.Println("a.PublicIpSubnet=", *a.PublicIpSubnet)
	}
	if a.Qcow2Image != nil {
		fmt.Println("a.Qcow2Image=", *a.Qcow2Image)
	}

	// ここでエラーになるのは、クライアントライブラリが未完のため
	if a.VmSpec != nil {
		for _, v := range *a.VmSpec {

			if v.Name != nil {
				fmt.Println("v.Name=", *v.Name)
			}
			if v.Cpu != nil {
				fmt.Println("v.Cpu=", int(*v.Cpu))
			}
			if v.Memory != nil {
				fmt.Println("v.Memory=", *v.Memory)
			}
			if v.PrivateIp != nil {
				fmt.Println("v.PrivateIp=", *v.PrivateIp)
			}
			if v.PublicIp != nil {
				fmt.Println("v.PublicIp=", *v.PublicIp)
			}
			if v.Comment != nil {
				fmt.Println("v.Comment=", *v.Comment)
			}
			if v.Key != nil {
				fmt.Println("v.Key=", *v.Key)
			}
			if v.Ostemplv != nil {
				fmt.Println("v.Ostemplv=", *v.Ostemplv)
			}
			if v.Ostempvg != nil {
				fmt.Println("v.Ostempvg=", *v.Ostempvg)
			}
			if v.Ostempvariant != nil {
				fmt.Println("v.Ostempvariant=", *v.Ostempvariant)
			}
			if v.Uuid != nil {
				fmt.Println("v.Uuid=", *v.Uuid)
			}
			if v.Playbook != nil {
				fmt.Println("v.Playbook=", *v.Playbook)
			}
			if v.Storage != nil {
				for _, v2 := range *v.Storage {
					if v2.Name != nil {
						fmt.Println("v2.Name=", *v2.Name)
					}
					if v2.Path != nil {
						fmt.Println("v2.Path=", *v2.Path)
					}
					if v2.Size != nil {
						fmt.Println("v2.Size=", int(*v2.Size))
					}
					if v2.Type != nil {
						fmt.Println("v2.Type=", *v2.Type)
					}
					if v2.Vg != nil {
						fmt.Println("v2.Vg=", *v2.Vg)
					}
				}
			}
		}
	}
	fmt.Println("=========================================")
}
