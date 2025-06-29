package db

func testVmData() VirtualMachine {
	var vm VirtualMachine
	vm.Name = "server1"
	vm.Cpu = 4
	vm.Memory = 24
	vm.PrivateIp = "172.16.0.100"
	vm.Playbook = "setup.yaml"
	vm.Comment = "Test Data Cluster "
	vm.Key = "vm001"
	vm.HvNode = "hv01"
	var st Storage
	st.Name = "log"
	st.Size = 10
	st.Path = "/stg2"
	vm.Storage = append(vm.Storage, st)
	st.Name = "data1"
	st.Size = 100
	st.Path = "/stg2"
	vm.Storage = append(vm.Storage, st)
	st.Name = "data2"
	st.Size = 100
	st.Path = "/stg2"
	vm.Storage = append(vm.Storage, st)
	st.Name = "data3"
	st.Size = 100
	st.Path = "/stg2"
	vm.Storage = append(vm.Storage, st)

	return vm
}

func testCreateVm() VirtualMachine {
	var vm VirtualMachine
	vm.Name = "node1-k8s1"
	vm.Cpu = 4
	vm.Memory = 24
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

func testHvData2() Hypervisor {
	var hv Hypervisor
	hv.Nodename = "hv02"
	hv.Cpu = 10
	hv.Memory = 64
	hv.IpAddr = "10.1.0.101"
	hv.FreeCpu = 10
	hv.FreeMemory = 64
	hv.Key = "hv02"

	return hv
}

func testHvCreate(name string, cpu int, ram int) Hypervisor {
	var hv Hypervisor
	hv.Nodename = name
	hv.Cpu = cpu
	hv.Memory = ram
	hv.IpAddr = "10.1.0.100"
	hv.FreeCpu = cpu
	hv.FreeMemory = ram
	hv.Key = name
	hv.Status = 2
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
