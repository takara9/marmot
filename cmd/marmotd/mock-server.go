package main

import (
	"fmt"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"

	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

var EtcdEndpoint string = "http://localhost:12379"
var NodeName string = "hvc"

type Server struct{}

func main() {
	e := echo.New()
	server := Server{}
	api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
	// And we serve HTTP until the world ends.
	fmt.Println(e.Start("0.0.0.0:8080"))
}

func (s Server) ReplyPing(ctx echo.Context) error {
	return ctx.JSON(200, api.ReplyMessage{Message: "ok"})
}

func (s Server) GetVersion(ctx echo.Context) error {
	return ctx.JSON(200, api.Version{Version: "0.0.1"})
}

// ListHypervisors implements api.ServerInterface.
func (s Server) ListHypervisors(ctx echo.Context, params api.ListHypervisorsParams) error {
	// リストの最大数の設定は、未実装
	//panic("unimplemented")
	// ハイパーバイザーの稼働チェック　結果はDBへ反映
	_, err := util.CheckHypervisors(EtcdEndpoint, NodeName)
	if err != nil {
		println("#1")
		return ctx.JSON(500, api.Error{Code: 500, Message: err.Error()})
	}

	// ストレージ容量の更新 結果はDBへ反映
	err = util.CheckHvVgAll(EtcdEndpoint, NodeName)
	if err != nil {
		println("#2")
		return ctx.JSON(500, api.Error{Code: 500, Message: err.Error()})
	}

	// データベースから情報を取得
	Conn, err := db.Connect(EtcdEndpoint)
	if err != nil {
		println("#3")
		return ctx.JSON(500, api.Error{Code: 500, Message: err.Error()})
	}

	var hvs []db.Hypervisor
	err = db.GetHvsStatus(Conn, &hvs)
	if err != nil {
		println("#4")
		return ctx.JSON(500, api.Error{Code: 500, Message: err.Error()})
	}

	return ctx.JSON(200, hvs)
}

func (s Server) ListVirtualMachines(ctx echo.Context) error {
	Conn, err := db.Connect(EtcdEndpoint)
	if err != nil {
		return ctx.JSON(500, api.Error{Code: 500, Message: err.Error()})
	}

	var vms []db.VirtualMachine
	err = db.GetVmsStatus(Conn, &vms)
	if err != nil {
		return ctx.JSON(500, api.Error{Code: 500, Message: err.Error()})
	}

	return ctx.JSON(200, vms)
}

func (s Server) CreateCluster(ctx echo.Context) error {
	var apix api.MarmotConfig

	fmt.Println("========== Create Cluster Accept Access ========")
	if err := ctx.Bind(&apix); err != nil {
		return ctx.JSON(400, api.Error{Code: 400, Message: "Invalid request body"})
	}

	fmt.Println("==== recv data =", *apix.Domain)
	cnf := convertToMarmotConfig(apix)
	printMarmotConf(cnf)

	// ハイパーバイザーの稼働チェック　結果はDBへ反映
	fmt.Println("=============1")
	_, err := util.CheckHypervisors(EtcdEndpoint, NodeName)
	if err != nil {
		return ctx.JSON(500, api.Error{Code: 500, Message: err.Error()})
	}
	fmt.Println("=============2")
	if err := util.CreateCluster(cnf, EtcdEndpoint, NodeName); err != nil {
		return ctx.JSON(500, api.Error{Code: 500, Message: err.Error()})
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

	fmt.Println("=================== CreateVirtualMachine xxxxxxxxxxxxxxxxxx 1")
	//var sc cf.VMSpec
	var vm api.VmSpec
	//var NodeName string = "hvc"

	if err := ctx.Bind(&vm); err != nil {
		return ctx.JSON(500, api.Error{Code: 500, Message: err.Error()})
	}

	fmt.Println("=================== CreateVirtualMachine xxxxxxxxxxxxxxxxxx 2")
	Conn, err := db.Connect(EtcdEndpoint)
	if err != nil {
		return ctx.JSON(500, api.Error{Code: 500, Message: "Connect DB"})
	}

	fmt.Println("=================== CreateVirtualMachine xxxxxxxxxxxxxxxxxx 3")
	sc := convertVMSpec(vm)

	fmt.Println("=================== CreateVirtualMachine xxxxxxxxxxxxxxxxxx 4")
	err = util.CreateVM(Conn, sc, NodeName)
	if err != nil {
		return ctx.JSON(500, api.Error{Code: 500, Message: "create virtual machine"})
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
		fmt.Printf("VM Name %d: %s\n", k, *v.Name)
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
		fmt.Printf("VM Name %d: %s\n", k, *v.Name)
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
