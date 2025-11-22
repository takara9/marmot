package db

import (
	"github.com/takara9/marmot/api"
	. "github.com/takara9/marmot/pkg/types"
)

func TestHvData1() api.Hypervisor {
	var hv api.Hypervisor
	hv.NodeName = "hv01"
	hv.Cpu = 10
	hv.Memory = int64PtrConvMB(64)
	hv.IpAddr = stringPtr("10.1.0.100")
	hv.FreeCpu = int32Ptr(10)
	hv.FreeMemory = int64PtrConvMB(64)
	hv.Key = stringPtr("hv01")
	return hv
}

func TestVmCreate(hostname string, cpu int, ram int) VirtualMachine {
	var vm VirtualMachine
	vm.Name = hostname
	vm.Cpu = cpu
	vm.Memory = ram
	vm.PrivateIp = "172.16.0.100"
	vm.PublicIp = "192.168.1.100"
	vm.Playbook = "setup.yaml"
	vm.Comment = "Test Data Cluster "

	var st Storage
	st.Name = "log"
	st.Size = 10
	st.Path = "/stg"
	vm.Storage = append(vm.Storage, st)
	st.Name = "data1"
	st.Size = 100
	st.Path = "/stg"
	vm.Storage = append(vm.Storage, st)
	st.Name = "data2"
	st.Size = 100
	st.Path = "/stg"
	vm.Storage = append(vm.Storage, st)
	st.Name = "data3"
	st.Size = 100
	st.Path = "/stg"
	vm.Storage = append(vm.Storage, st)

	return vm
}
