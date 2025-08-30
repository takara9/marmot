package db

func testHvData1() Hypervisor {
	var hv Hypervisor
	hv.Nodename = "hv01"
	hv.Cpu = 10
	hv.Memory = 64
	hv.IpAddr = "10.1.0.100"
	hv.FreeCpu = 10
	hv.FreeMemory = 64
	hv.Key = "hv01"
	return hv
}

func testVmCreate(hostname string, cpu int, ram int) VirtualMachine {
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
