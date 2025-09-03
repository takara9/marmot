package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

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
	fmt.Println("ListVirtualMachines")
	return ctx.JSON(200, "")
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
