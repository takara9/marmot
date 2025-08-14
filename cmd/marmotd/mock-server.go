package main

import (
	"fmt"
	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"

	cf "github.com/takara9/marmot/pkg/config"
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

func convertToMarmotConfig(apix api.MarmotConfig) cf.MarmotConfig {
	var cnf cf.MarmotConfig
	// OpenAPIの構造体から、内部の設定構造体に変換
	cnf.Domain = *apix.Domain
	cnf.ClusterName = *apix.ClusterName
	cnf.Hypervisor = *apix.Hypervisor
	cnf.VmImageTempPath = *apix.ImgaeTemplatePath
	cnf.VMImageDfltPath = *apix.ImgaeTemplatePath
	cnf.VMImageQCOW = *apix.Qcow2Image
	cnf.VMOsVariant = *apix.OsVariant
	cnf.NetDevDefault = *apix.NetDevDefault
	cnf.NetDevPrivate = *apix.NetDevPrivate
	cnf.NetDevPublic = *apix.NetDevPublic
	cnf.PublicIPGw = *apix.PublicIpGw
	cnf.PublicIPDns = *apix.PublicIpSubnet
	cnf.PublicIPSubnet = *apix.PublicIpSubnet
	cnf.PrivateIPSubnet = *apix.PrivateIpSubnet
	cnf.VMSpec = make([]cf.VMSpec, len(*apix.VmSpec))
	for k, v := range *apix.VmSpec {
		cnf.VMSpec[k].Name = *v.Name
		cnf.VMSpec[k].CPU = int(*v.Cpu)
		cnf.VMSpec[k].Memory = int(*v.Memory)
		cnf.VMSpec[k].PrivateIP = *v.PrivateIp
		cnf.VMSpec[k].PublicIP = *v.PublicIp
		cnf.VMSpec[k].Storage = make([]cf.Storage, len(*v.Storage))
		for k2, v2 := range *v.Storage {
			cnf.VMSpec[k].Storage[k2].Name = *v2.Name
			cnf.VMSpec[k].Storage[k2].Size = int(*v2.Size)
			cnf.VMSpec[k].Storage[k2].Path = *v2.Path
			cnf.VMSpec[k].Storage[k2].VolGrp = *v2.Vg
			cnf.VMSpec[k].Storage[k2].Type = *v2.Type
		}
	}
	return cnf
}

func (s Server) CreateCluster(ctx echo.Context) error {
	var apix api.MarmotConfig

	fmt.Println("========== Create Cluster Accept Access ========")
	if err := ctx.Bind(&apix); err != nil {
		return ctx.JSON(400, api.Error{Code: 400, Message: "Invalid request body"})
	}
	cnf := convertToMarmotConfig(apix)

	/*	// etcdの設定を取得
		if EtcdEndpoint == "" || NodeName == "" {
			return ctx.JSON(400, api.Error{Code: 400, Message: "Node or etcd configuration is not set"})
		}

		// ハイパーバイザーの稼働チェック 結果はDBへ反映
		_, err := util.CheckHypervisors(EtcdEndpoint, NodeName)
		if err != nil {
			return ctx.JSON(400, api.Error{Code: 400, Message: err.Error()})
		}

		if err := util.CreateCluster(cnf, EtcdEndpoint, NodeName); err != nil {
			return ctx.JSON(400, api.Error{Code: 400, Message: err.Error()})
		}

		return ctx.JSON(201, "")
	*/

	//var cnf cf.MarmotConfig
	//if err := c.BindJSON(&cnf); err != nil {
	//	slog.Error("create vm cluster", "err", err)
	//	c.JSON(400, gin.H{"msg": err.Error()})
	//	return
	//}

	// ハイパーバイザーの稼働チェック　結果はDBへ反映
	_, err := util.CheckHypervisors(EtcdEndpoint, NodeName)
	if err != nil {
		return ctx.JSON(500, api.Error{Code: 500, Message: err.Error()})
	}

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
	var sc cf.VMSpec
	var vm api.VmSpec
	var NodeName string = "hvc"

	if err := ctx.Bind(&vm); err != nil {
		return ctx.JSON(500, api.Error{Code: 500, Message: err.Error()})
	}

	if vm.Name != nil {
		sc.Name = *vm.Name
	}

	if vm.Cpu != nil {
		sc.CPU = int(*vm.Cpu)
	}

	if vm.Memory != nil {
		sc.Memory = int(*vm.Memory)
	}

	if vm.PrivateIp != nil {
		sc.PrivateIP = *vm.PrivateIp
	}

	if vm.PublicIp != nil {
		sc.PublicIP = *vm.PublicIp
	}

	if vm.Ostempvg != nil {
		sc.OsTempVg = *vm.Ostempvg
	}

	if vm.Ostemplv != nil {
		sc.OsTempLv = *vm.Ostemplv
	}

	if vm.Key != nil {
		sc.Key = *vm.Key
	}

	if vm.Storage != nil {
		sc.Storage = make([]cf.Storage, len(*vm.Storage))
		for k, v := range *vm.Storage {
			fmt.Printf("VM Storage %s: 	%s, Size: %d\n", *v.Vg, *v.Name, *v.Size)
			sc.Storage[k].Name = *v.Name
			sc.Storage[k].Size = int(*v.Size)
			//sc.Storage[k].Path = *v.Path
			fmt.Println("VVVG Name=", *v.Vg)
			sc.Storage[k].VolGrp = *v.Vg
			//sc.Storage[k].Type = *v.Type
		}
	}
	slog.Info("create vm", "etcd", EtcdEndpoint)

	Conn, err := db.Connect(EtcdEndpoint)
	if err != nil {
		return ctx.JSON(500, api.Error{Code: 500, Message: "Connect DB"})
	}

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
