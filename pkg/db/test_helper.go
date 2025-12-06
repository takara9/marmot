package db

import (
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestHvData1() api.Hypervisor {
	var hv api.Hypervisor
	hv.NodeName = "hv01"
	hv.Cpu = 10
	hv.Memory = util.Int64PtrConvMB(64)
	hv.IpAddr = util.StringPtr("10.1.0.100")
	hv.FreeCpu = util.Int64PtrInt32(10)
	hv.FreeMemory = util.Int64PtrConvMB(64)
	hv.Key = util.StringPtr("hv01")
	return hv
}

func TestVmCreate(hostname string, cpu int, ram int) api.VirtualMachine {
	var vm api.VirtualMachine
	vm.Name = hostname
	vm.Cpu = util.IntPtrInt32(cpu)
	vm.Memory = util.IntPtrInt64(ram)
	vm.PrivateIp = util.StringPtr("172.16.0.100")
	vm.PublicIp = util.StringPtr("192.168.1.100")
	vm.Playbook = util.StringPtr("setup.yaml")
	vm.Comment = util.StringPtr("Test Data Cluster ")

	var sta []api.Storage
	var st api.Storage

	st.Name = util.StringPtr("log")
	st.Size = util.IntPtrInt64(10)
	st.Path = util.StringPtr("/stg")
	sta = append(sta, st)

	st.Name = util.StringPtr("data1")
	st.Size = util.IntPtrInt64(100)
	st.Path = util.StringPtr("/stg")
	sta = append(sta, st)

	st.Name = util.StringPtr("data2")
	st.Size = util.IntPtrInt64(100)
	st.Path = util.StringPtr("/stg")
	sta = append(sta, st)

	st.Name = util.StringPtr("data3")
	st.Size = util.IntPtrInt64(100)
	st.Path = util.StringPtr("/stg")
	sta = append(sta, st)
	vm.Storage = &sta

	return vm
}
