package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmot"
	"github.com/takara9/marmot/pkg/util"
)

// Echoの実装の構造体、レシーバーとして利用できなさそう
type Server struct{}

// コールバックで参照できるようにMarmotのインスタンスをグローバルに持つ
var mx *marmot.Marmot

func main() {
	var err error
	node := flag.String("node", "hv1", "Hypervisor node name")
	etcd := flag.String("etcd", "http://127.0.0.1:3379", "etcd url")
	flag.Parse()
	fmt.Println("node = ", *node)
	fmt.Println("etcd = ", *etcd)

	// Setup slog
	opts := &slog.HandlerOptions{
		AddSource: true,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	mx, err = marmot.NewMarmot(*node, *etcd)
	if err != nil {
		slog.Error("Storage free space check", "err", err)
		os.Exit(1)
	}

	e := echo.New()
	server := Server{}
	api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
	// And we serve HTTP until the world ends.
	fmt.Println(e.Start("0.0.0.0:8080"))
}

func (s Server) ReplyPing(ctx echo.Context) error {
	return ctx.JSON(http.StatusOK, api.ReplyMessage{Message: "ok"})
}

func (s Server) GetVersion(ctx echo.Context) error {
	return ctx.JSON(http.StatusOK, api.Version{Version: "0.0.1"})
}

func (s Server) ListHypervisors(ctx echo.Context, params api.ListHypervisorsParams) error {
	_, err := util.CheckHypervisors(mx.EtcdUrl, mx.NodeName)
	if err != nil {
		slog.Error("Check if the hypervisor is up and running", "err", err)
		return ctx.JSON(http.StatusInternalServerError, nil)
	}

	// ストレージ容量の更新 結果はDBへ反映
	err = util.CheckHvVgAll(mx.EtcdUrl, mx.NodeName)
	if err != nil {
		slog.Error("Update storage capacity", "err", err)
		return ctx.JSON(http.StatusInternalServerError, nil)
	}

	// データベースから情報を取得
	d, err := db.NewDatabase(mx.EtcdUrl)
	if err != nil {
		slog.Error("connect to database", "err", err)
		return ctx.JSON(http.StatusInternalServerError, nil)
	}
	var hvs []db.Hypervisor
	err = d.GetHvsStatus(&hvs)
	if err != nil {
		slog.Error("get hypervisor status", "err", err)
		return ctx.JSON(http.StatusInternalServerError, nil)
	}
	return ctx.JSON(http.StatusOK, hvs)
}

func (s Server) ListVirtualMachines(ctx echo.Context) error {
	fmt.Println("========== ListVirtualMachines ===========")
	d, err := db.NewDatabase(mx.EtcdUrl)
	if err != nil {
		slog.Error("get list virtual machines", "err", err)
		return ctx.JSON(http.StatusInternalServerError, nil)
	}

	var vms []db.VirtualMachine
	err = d.GetVmsStatus(&vms)
	if err != nil {
		slog.Error("get status of virtual machines", "err", err)
		return ctx.JSON(http.StatusInternalServerError, nil)
	}

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

		/*
			Storage     *[]Storage `json:"storage,omitempty"`
		*/

		/*
			var disk []api.Disk
			for _, d := range vm.Disk {
				disk = append(disk, api.Disk{
					DiskId:   &d.DiskId,
					Capacity: int64(d.Capacity),
					Type:     &d.Type,
					Bus:      &d.Bus,
				})
			}
			var nic []api.Nic
			for _, n := range vm.Nic {
				nic = append(nic, api.Nic{
					NicId: &n.NicId,
					//Mac:    &n.M
					Bridge: &n.Bridge,
					IpAddr: &n.IpAddr,
					Type:   &n.Type,
				})
			}
		*/

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
	return ctx.JSON(http.StatusOK, vms2)
}

func (s Server) CreateCluster(ctx echo.Context) error {
	return ctx.JSON(201, nil)
}

func (s Server) DestroyCluster(ctx echo.Context) error {
	return ctx.JSON(200, "")
}

func (s Server) CreateVirtualMachine(ctx echo.Context) error {
	return ctx.JSON(201, "")
}

func (s Server) DestroyVirtualMachine(ctx echo.Context) error {
	return ctx.JSON(201, "")
}

func (s Server) StopCluster(ctx echo.Context) error {
	return ctx.JSON(200, "")
}

func (s Server) StopVirtualMachine(ctx echo.Context) error {
	return ctx.JSON(200, "")
}

func (s Server) StartCluster(ctx echo.Context) error {
	return ctx.JSON(200, "")
}

func (s Server) StartVirtualMachine(ctx echo.Context) error {
	return ctx.JSON(200, "")
}

func (s Server) ShowHypervisorById(ctx echo.Context, hypervisorId string) error {
	var hvs []db.Hypervisor
	var hvs2 api.Hypervisor
	err := mx.Db.GetHvsStatus(&hvs)
	if err != nil {
		slog.Error("get hypervisor status", "err", err)
		return ctx.JSON(http.StatusInternalServerError, nil)
	}

	if len(hvs) < 1 {
		slog.Error("No such hypervisor", "id", hypervisorId)
		return ctx.JSON(http.StatusInternalServerError, nil)
	}

	for _, v := range hvs {
		if hypervisorId == v.Nodename {
			var memory int64 = int64(hvs[0].Memory)
			var ipaddr string = hvs[0].IpAddr
			var freecpu int32 = int32(hvs[0].FreeCpu)
			var freememory int64 = int64(hvs[0].FreeMemory)
			var status int32 = int32(hvs[0].Status)
			var stgpool []api.StoragePool
			for _, v := range hvs[0].StgPool {
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
			hvs2 = api.Hypervisor{
				NodeName:   hvs[0].Nodename,
				IpAddr:     &ipaddr,
				Cpu:        int32(hvs[0].Cpu),
				FreeCpu:    &freecpu,
				Memory:     &memory,
				FreeMemory: &freememory,
				Status:     &status,
				StgPool:    &stgpool,
			}
			return ctx.JSON(http.StatusOK, hvs2)
		}
	}
	return ctx.JSON(http.StatusNotFound, api.ReplyMessage{Message: "Hypervisor " + hypervisorId + " not found"})
}

func (s Server) CreateVmCluster(ctx echo.Context) error {
	return ctx.JSON(200, nil)
}

func (s Server) DeleteVmCluster(ctx echo.Context) error {
	return ctx.JSON(200, nil)
}

func (s Server) ListVmClusters(ctx echo.Context) error {
	return ctx.JSON(200, nil)
}

func (s Server) UpdateVmCluster(ctx echo.Context) error {
	return ctx.JSON(200, nil)
}
