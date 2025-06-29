package main

import (
	"flag"
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"

	cf "github.com/takara9/marmot/pkg/config"
	db "github.com/takara9/marmot/pkg/db"
	ut "github.com/takara9/marmot/pkg/util"
)

// ローカルノード
var node *string
var etcd *string

func main() {

	// 起動パラメータ
	node = flag.String("node", "hv1", "Hypervisor node name")
	etcd = flag.String("etcd", "http://127.0.0.1:2379", "etcd url")

	flag.Parse()

	fmt.Println("node = ", *node)
	fmt.Println("etcd = ", *etcd)

	// Setup slog
	opts := &slog.HandlerOptions{
		AddSource: true,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	// 起動チェック ストレージの空き容量チェック
	err := ut.CheckHvVgAll(*etcd, *node)
	if err != nil {
		slog.Error("Storage free space check", "err", err)
		os.Exit(1)
	}

	// REST-APIサーバー
	router := gin.Default()

	// 状態取得
	router.GET("/ping", accessTest)
	router.GET("/hypervisors", listHypervisor)
	router.GET("/virtualMachines", listVirtualMachines)

	// マスター処理
	router.POST("/createCluster", createCluster)
	router.POST("/destroyCluster", destroyCluster)
	router.POST("/createVm", createVm)
	router.POST("/destroyVm", destroyVm)

	// リモート処理
	router.POST("/stopCluster", stopCluster)
	router.POST("/stopVm", stopVm)
	router.POST("/startCluster", startCluster)
	router.POST("/startVm", startVm)

	// サーバー待機
	router.Run("0.0.0.0:8750")
}

// コールバック アクセステスト用
func accessTest(c *gin.Context) {
	// チェック機能を追加して、最終的にOK/NGを返す
	c.JSON(200, gin.H{"message": "ok"})
}

// コールバック ハイパーバイザーの状態取得
func listHypervisor(c *gin.Context) {

	// ハイパーバイザーの稼働チェック　結果はDBへ反映
	_, err := ut.CheckHypervisors(*etcd, *node)
	if err != nil {
		slog.Error("Check if the hypervisor is up and running", "err", err)
		return
	}

	// ストレージ容量の更新 結果はDBへ反映
	err = ut.CheckHvVgAll(*etcd, *node)
	if err != nil {
		slog.Error("Update storage capacity", "err", err)
		return
	}

	// データベースから情報を取得
	Conn, err := db.Connect(*etcd)
	if err != nil {
		slog.Error("connect to database", "err", err)
		return
	}

	var hvs []db.Hypervisor
	err = db.GetHvsStatus(Conn, &hvs)
	if err != nil {
		slog.Error("get hypervisor status", "err", err)
		return
	}
	c.IndentedJSON(http.StatusOK, hvs)
}

// コールバック 仮想マシンのリスト
func listVirtualMachines(c *gin.Context) {
	Conn, err := db.Connect(*etcd)
	if err != nil {
		slog.Error("get list virtual machines", "err", err)
		return
	}
	var vms []db.VirtualMachine
	err = db.GetVmsStatus(Conn, &vms)
	if err != nil {
		slog.Error("get status of virtual machines", "err", err)
		return
	}
	c.IndentedJSON(http.StatusOK, vms)
}

// コールバック VMクラスタの作成
func createCluster(c *gin.Context) {

	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("create vm cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}

	// ハイパーバイザーの稼働チェック　結果はDBへ反映
	_, err := ut.CheckHypervisors(*etcd, *node)
	if err != nil {
		slog.Error("check hypervisor status", "err", err)
		return
	}

	if err := ut.CreateCluster(cnf, *etcd, *node); err != nil {
		slog.Error("create cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// コールバック VMクラスタの削除
func destroyCluster(c *gin.Context) {

	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("prepare to delete cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	fmt.Println(cnf)

	if err := ut.DestroyCluster(cnf, *etcd); err != nil {
		slog.Error("delete cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// VMの作成
func createVm(c *gin.Context) {
	slog.Info("create vm", "etcd", *etcd)

	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("create vm in action", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}

	Conn, err := db.Connect(*etcd)
	if err != nil {
		slog.Error("connectiong db", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}

	err = ut.CreateVM(Conn, spec, *node)
	if err != nil {
		slog.Error("creating vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// VMの削除
func destroyVm(c *gin.Context) {
	slog.Info("destroy vm", "etcd", *etcd)

	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup spec", "err", err)
		return
	}

	Conn, err := db.Connect(*etcd)
	if err != nil {
		slog.Error("connecting to Db", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return

	}
	err = ut.DestroyVM(Conn, spec, *node)
	if err != nil {
		slog.Error("delete vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// クラスタの停止
func stopCluster(c *gin.Context) {
	slog.Info("stop cluster", "etcd", *etcd)

	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("setup Json", "err", err)
		c.JSON(400, gin.H{"msg": "Can't read JSON"})
		return
	}
	if err := ut.StopCluster(cnf, *etcd); err != nil {
		slog.Error("stop cluster", "err", err)
		return
	}
}

// クラスタの再スタート
func startCluster(c *gin.Context) {
	slog.Info("start cluster", "etcd", *etcd)

	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("setup config", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	if err := ut.StartCluster(cnf, *etcd); err != nil {
		slog.Error("start cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// 仮想マシンの開始
func startVm(c *gin.Context) {
	slog.Info("start vm", "etcd", *etcd)

	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup config", "err", err)
		return
	}

	Conn, err := db.Connect(*etcd)
	if err != nil {
		slog.Error("setup config", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	err = ut.StartVM(Conn, spec)
	if err != nil {
		slog.Error("start vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// 仮想マシンの停止
func stopVm(c *gin.Context) {
	slog.Info("stop vm", "etcd", *etcd)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup config", "err", err)
		return
	}

	Conn, err := db.Connect(*etcd)
	if err != nil {
		slog.Error("connecting db", "err", err, "etcd = ", *etcd)
		c.JSON(400, gin.H{"msg": err.Error()})
		return

	}
	err = ut.StopVM(Conn, spec)
	if err != nil {
		slog.Error("stop vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}
