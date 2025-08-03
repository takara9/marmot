package main

import (
	"fmt"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

type Server struct{}

func main() {
	e := echo.New()
	server := Server{}
	//api.RegisterHandlers(e, server)
	api.RegisterHandlersWithBaseURL(e, server, "/api/v1")

	// And we serve HTTP until the world ends.
	fmt.Println(e.Start("0.0.0.0:8080"))
}

func (s Server) GetPong(ctx echo.Context) error {
	return ctx.JSON(200, api.Pong{Ping: "OK"})
}

func (s Server) GetVersion(ctx echo.Context) error {
	return ctx.JSON(200, api.Version{Version: "0.0.1"})
}

// ListHypervisors implements api.ServerInterface.
func (s Server) ListHypervisors(ctx echo.Context, params api.ListHypervisorsParams) error {
	//panic("unimplemented")
	//fmt.Printf("ListHypervisors called with params: %+v\n", params)

	//fmt.Println("ListHypervisors called with params:", *params.Limit)

	//if params.Limit != nil && *params.Limit > 100 {
	//	return ctx.JSON(http.StatusBadRequest, "Limit cannot exceed 100")
	//}

	// Example response with two hypervisors
	// In a real application, this would likely come from a database or other data source.
	// Here we just return a static list for demonstration purposes.
	IpAddr1 := "127.0.0.1"
	//IpAddr2 := "127.0.0.5"
	var memory int64
	memory = 1024 * 1024 * 1024 // 1 GB in bytes

	hvs := api.Hypervisors{
		{NodeName: "hv1", Cpu: 64, IpAddr: &IpAddr1},
		{NodeName: "hv2", Cpu: 64, Memory: &memory},
	}
	return ctx.JSON(200, hvs)
}

func (s Server) ListVirtualMachines(ctx echo.Context) error {

	vms := api.VirtualMachines{
		{
			Name:   "vm1",
			HvNode: "hv1",
		},
		{
			Name:   "vm2",
			HvNode: "hv1",
		},
	}
	return ctx.JSON(200, vms)
}

func (s Server) CreateCluster(ctx echo.Context) error {
	var cluster api.MarmotConfig
	if err := ctx.Bind(&cluster); err != nil {
		return ctx.JSON(400, api.Error{Code: 400, Message: "Invalid request body"})
	}
	fmt.Printf("CreateCluster called with config: %s\n", *cluster.Domain)
	fmt.Printf("OsVariant: %s\n", *cluster.OsVariant)
	for k, v := range *cluster.VmSpec {
		fmt.Printf("VM Name %d: %s\n", k, *v.Name)
		fmt.Printf("VM CPU %d: %d\n", k, *v.Cpu)
		fmt.Printf("VM Memory %d: %d\n", k, *v.Memory)
		for k2, v2 := range *v.Storage {
			fmt.Printf("VM Storage %d: %s, Size: %d\n", k2, *v2.Name, *v2.Size)
		}
	}
	return ctx.JSON(201, "")
}

func (s Server) DestroyCluster(ctx echo.Context) error {
	var cluster api.MarmotConfig
	if err := ctx.Bind(&cluster); err != nil {
		return ctx.JSON(400, api.Error{Code: 400, Message: "Invalid request body"})
	}
	fmt.Printf("CreateCluster called with config: %s\n", *cluster.Domain)
	fmt.Printf("OsVariant: %s\n", *cluster.OsVariant)
	for k, v := range *cluster.VmSpec {
		fmt.Printf("VM Name %d: %s\n", k, *v.Name)
		fmt.Printf("VM CPU %d: %d\n", k, *v.Cpu)
		fmt.Printf("VM Memory %d: %d\n", k, *v.Memory)
		for k2, v2 := range *v.Storage {
			fmt.Printf("VM Storage %d: %s, Size: %d\n", k2, *v2.Name, *v2.Size)
		}
	}
	return ctx.JSON(200, "")
}
func (s Server) CreateVirtualMachine(ctx echo.Context) error {
	var vm api.VirtualMachine
	if err := ctx.Bind(&vm); err != nil {
		return ctx.JSON(400, api.Error{Code: 400, Message: "Invalid request body"})
	}
	fmt.Printf("VM Name %s\n", vm.Name)
	fmt.Printf("VM CPU %d\n", *vm.Cpu)
	fmt.Printf("VM Memory %d\n", *vm.Memory)
	for k, v := range *vm.Storage {
		fmt.Printf("VM Storage %d: %s, Size: %d\n", k, *v.Name, *v.Size)
	}
	return ctx.JSON(201, "")
}

func (s Server) DestroyVirtualMachine(ctx echo.Context) error {
	var vm api.VirtualMachine
	if err := ctx.Bind(&vm); err != nil {
		return ctx.JSON(400, api.Error{Code: 400, Message: "Invalid request body"})
	}
	fmt.Printf("VM Name %s\n", vm.Name)
	fmt.Printf("VM CPU %d\n", *vm.Cpu)
	fmt.Printf("VM Memory %d\n", *vm.Memory)
	for k, v := range *vm.Storage {
		fmt.Printf("VM Storage %d: %s, Size: %d\n", k, *v.Name, *v.Size)
	}
	return ctx.JSON(200, "")
}

func (s Server) StopCluster(ctx echo.Context) error {
	var cluster api.MarmotConfig
	if err := ctx.Bind(&cluster); err != nil {
		return ctx.JSON(400, api.Error{Code: 400, Message: "Invalid request body"})
	}
	fmt.Printf("CreateCluster called with config: %s\n", *cluster.Domain)
	fmt.Printf("OsVariant: %s\n", *cluster.OsVariant)
	for k, v := range *cluster.VmSpec {
		fmt.Printf("VM Name %d: %s\n", k, v.Name)
		fmt.Printf("VM CPU %d: %d\n", k, *v.Cpu)
		fmt.Printf("VM Memory %d: %d\n", k, *v.Memory)
		for k2, v2 := range *v.Storage {
			fmt.Printf("VM Storage %d: %s, Size: %d\n", k2, *v2.Name, *v2.Size)
		}
	}
	return ctx.JSON(200, "")
}

func (s Server) StopVirtualMachine(ctx echo.Context) error {
	var vm api.VirtualMachine
	if err := ctx.Bind(&vm); err != nil {
		return ctx.JSON(400, api.Error{Code: 400, Message: "Invalid request body"})
	}
	fmt.Printf("VM Name %s\n", vm.Name)
	fmt.Printf("VM CPU %d\n", *vm.Cpu)
	fmt.Printf("VM Memory %d\n", *vm.Memory)
	for k, v := range *vm.Storage {
		fmt.Printf("VM Storage %d: %s, Size: %d\n", k, *v.Name, *v.Size)
	}
	return ctx.JSON(200, "")
}

func (s Server) StartCluster(ctx echo.Context) error {
	return ctx.JSON(200, api.Version{Version: "1.0.0"})
	var cluster api.MarmotConfig
	if err := ctx.Bind(&cluster); err != nil {
		return ctx.JSON(400, api.Error{Code: 400, Message: "Invalid request body"})
	}
	fmt.Printf("CreateCluster called with config: %s\n", *cluster.Domain)
	fmt.Printf("OsVariant: %s\n", *cluster.OsVariant)
	for k, v := range *cluster.VmSpec {
		fmt.Printf("VM Name %d: %s\n", k, v.Name)
		fmt.Printf("VM CPU %d: %d\n", k, *v.Cpu)
		fmt.Printf("VM Memory %d: %d\n", k, *v.Memory)
		for k2, v2 := range *v.Storage {
			fmt.Printf("VM Storage %d: %s, Size: %d\n", k2, *v2.Name, *v2.Size)
		}
	}
	return ctx.JSON(200, "")
}
func (s Server) StartVirtualMachine(ctx echo.Context) error {
	var vm api.VirtualMachine
	if err := ctx.Bind(&vm); err != nil {
		return ctx.JSON(400, api.Error{Code: 400, Message: "Invalid request body"})
	}
	fmt.Printf("VM Name %s\n", vm.Name)
	fmt.Printf("VM CPU %d\n", *vm.Cpu)
	fmt.Printf("VM Memory %d\n", *vm.Memory)
	for k, v := range *vm.Storage {
		fmt.Printf("VM Storage %d: %s, Size: %d\n", k, *v.Name, *v.Size)
	}
	return ctx.JSON(200, "")
}

func (s Server) ShowHypervisorById(ctx echo.Context, hypervisorId string) error {
	fmt.Printf("ShowHypervisorById called with ID: %s\n", hypervisorId)
	hv := api.Hypervisor{NodeName: "hv1", Cpu: 64}
	return ctx.JSON(200, hv)
}
func (s Server) CreateVmCluster(ctx echo.Context) error {
	return ctx.JSON(200, api.Version{Version: "1.0.0"})
}
func (s Server) DeleteVmCluster(ctx echo.Context) error {
	return ctx.JSON(200, api.Version{Version: "1.0.0"})
}
func (s Server) ListVmClusters(ctx echo.Context) error {
	return ctx.JSON(200, api.Version{Version: "1.0.0"})
}
func (s Server) UpdateVmCluster(ctx echo.Context) error {
	return ctx.JSON(200, api.Version{Version: "1.0.0"})
}
