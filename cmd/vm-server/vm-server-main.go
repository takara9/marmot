package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/takara9/marmot/pkg/marmot"
	ut "github.com/takara9/marmot/pkg/util"
)

func main() {
	// 起動パラメータ
	node := flag.String("node", "hv1", "Hypervisor node name")
	etcd := flag.String("etcd", "http://127.0.0.1:2379", "etcd url")
	flag.Parse()
	fmt.Println("node = ", *node)
	fmt.Println("etcd = ", *etcd)

	// Setup slog
	opts := &slog.HandlerOptions{
		AddSource: true,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	m, err := marmot.NewMarmot(*node, *etcd)
	if err != nil {
		slog.Error("Storage free space check", "err", err)
		os.Exit(1)
	}

	// 起動チェック ストレージの空き容量チェック
	err = ut.CheckHvVgAll(m.EtcdUrl, m.NodeName)
	if err != nil {
		slog.Error("Storage free space check", "err", err)
		os.Exit(1)
	}

	// REST-APIサーバー
	router := gin.Default()

	// 状態取得
	router.GET("/ping", m.AccessTest)                     //OK
	router.GET("/hypervisors", m.ListHypervisor)          //OK
	router.GET("/virtualMachines", m.ListVirtualMachines) //

	// マスター処理
	router.POST("/createCluster", m.CreateCluster)   //OK
	router.POST("/destroyCluster", m.DestroyCluster) //OK
	router.POST("/createVm", m.CreateVm)             //OK
	router.POST("/destroyVm", m.DestroyVm)           //OK

	// リモート処理
	router.POST("/stopCluster", m.StopCluster)
	router.POST("/stopVm", m.StopVm)
	router.POST("/startCluster", m.StartCluster)
	router.POST("/startVm", m.StartVm)

	// サーバー待機
	router.Run("0.0.0.0:8750")
}
