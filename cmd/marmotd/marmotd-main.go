package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	controller_net "github.com/takara9/marmot/pkg/contorller-net"
	controller_vm "github.com/takara9/marmot/pkg/controller-vm"
	controller_vol "github.com/takara9/marmot/pkg/controller-vol"
	"github.com/takara9/marmot/pkg/marmotd"
)

var controllerCounter uint64 = 0

// 定期的に実行したい関数
//func dispatchJobTask() {
//	slog.Info("ジョブの要求チェックと実行", "JOB", 0)
//}

//func startDispatcher() {
//	ticker := time.NewTicker(15 * time.Second)
// TODO: 停止処理を加えること
//	go func() {
//		for {
//			select {
//			case <-ticker.C:
//				dispatchJobTask()
//			}
//		}
//	}()
//}

func main() {
	// Setup slog
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	node := flag.String("node", "hv1", "Hypervisor node name")
	etcd := flag.String("etcd", "http://127.0.0.1:3379", "etcd url")
	flag.Parse()

	// REST-APIサーバーの処理
	e := echo.New()
	Server := marmotd.NewServer(*node, *etcd)
	api.RegisterHandlersWithBaseURL(e, Server, "/api/v1")

	// コントローラーの開始

	// 仮想マシンコントローラー
	_, err := controller_vm.StartVmController(*node, *etcd) // VMコントローラーの開始
	if err != nil {
		slog.Error("Failed to start controller", "err", err)
		return
	}
	// ボリュームコントローラー
	_, err = controller_vol.StartVolController(*node, *etcd) // ボリュームコントローラーの開始
	if err != nil {
		slog.Error("Failed to start controller", "err", err)
		return
	}

	// ネットワークコントローラー
	_, err = controller_net.StartNetController(*node, *etcd) // ネットワークコントローラーの開始
	if err != nil {
		slog.Error("Failed to start controller", "err", err)
		return
	}

	//startDispatcher()
	// And we serve HTTP until the world ends.
	fmt.Println(e.Start("0.0.0.0:8750"))
}
