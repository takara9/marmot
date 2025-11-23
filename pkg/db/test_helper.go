package db

import (
	"github.com/takara9/marmot/api"
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

func TestVmCreate(hostname string, cpu int, ram int) api.VirtualMachine {
	var vm api.VirtualMachine
	vm.Name = hostname
	vm.Cpu = intPtr(cpu)
	vm.Memory = int64Ptr(ram)
	vm.PrivateIp = stringPtr("172.16.0.100")
	vm.PublicIp = stringPtr("192.168.1.100")
	vm.Playbook = stringPtr("setup.yaml")
	vm.Comment = stringPtr("Test Data Cluster ")

	var sta []api.Storage
	var st api.Storage

	st.Name = stringPtr("log")
	st.Size = int64Ptr(10)
	st.Path = stringPtr("/stg")
	sta = append(sta, st)

	st.Name = stringPtr("data1")
	st.Size = int64Ptr(100)
	st.Path = stringPtr("/stg")
	sta = append(sta, st)

	st.Name = stringPtr("data2")
	st.Size = int64Ptr(100)
	st.Path = stringPtr("/stg")
	sta = append(sta, st)

	st.Name = stringPtr("data3")
	st.Size = int64Ptr(100)
	st.Path = stringPtr("/stg")
	sta = append(sta, st)
	vm.Storage = &sta

	return vm
}
