package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	cf "github.com/takara9/marmot/pkg/config"
	db "github.com/takara9/marmot/pkg/db"
)

// ローカルノード
var node *string
var etcd *string

type Marmotd struct {
	dbc      db.Db
	NodeName string
}

func NewMarmotd(etcdUrl string, nodeName string) (Marmotd, error) {
	var m Marmotd
	dbc, err := db.NewEtcdEp(etcdUrl)
	if err != nil {
		slog.Error("faild to create endpoint of etcd database", "err", err)
		os.Exit(1)
	}
	m.dbc = dbc
	return m, nil
}

func main() {
	// 起動パラメータ
	node = flag.String("node", "hv1", "Hypervisor node name")
	etcd = flag.String("etcd", "http://127.0.0.1:2379", "etcd url")
	flag.Parse()

	// Setup slog
	opts := &slog.HandlerOptions{
		AddSource: true,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	marmot, err := NewMarmotd(*etcd, *node)

	// 起動チェック ストレージの空き容量チェック
	err = marmot.CheckHvVgAll()
	if err != nil {
		slog.Error("Storage free space check", "err", err)
		os.Exit(1)
	}

	// REST-APIサーバー
	router := gin.Default()

	// 状態取得
	router.GET("/ping", marmot.accessTest)
	router.GET("/hypervisors", marmot.listHypervisor)
	router.GET("/virtualMachines", marmot.listVirtualMachines)

	// マスター処理
	router.POST("/createCluster", marmot.createCluster)
	router.POST("/destroyCluster", marmot.destroyCluster)
	router.POST("/createVm", marmot.createVm)
	router.POST("/destroyVm", marmot.destroyVm)

	// リモート処理
	router.POST("/stopCluster", marmot.stopCluster)
	router.POST("/stopVm", marmot.stopVm)
	router.POST("/startCluster", marmot.startCluster)
	router.POST("/startVm", marmot.startVm)

	// サーバー待機
	router.Run("0.0.0.0:8750")
}

// コールバック アクセステスト用
func (m *Marmotd) accessTest(c *gin.Context) {
	// チェック機能を追加して、最終的にOK/NGを返す
	c.JSON(200, gin.H{"message": "ok"})
}

// コールバック ハイパーバイザーの状態取得
func (m *Marmotd) listHypervisor(c *gin.Context) {
	// ハイパーバイザーの稼働チェック　結果はDBへ反映
	_, err := m.CheckHypervisors()
	if err != nil {
		slog.Error("Check if the hypervisor is up and running", "err", err)
		return
	}

	// ストレージ容量の更新 結果はDBへ反映
	err = m.CheckHvVgAll()
	if err != nil {
		slog.Error("Update storage capacity", "err", err)
		return
	}

	var hvs []db.Hypervisor
	err = m.dbc.GetHvsStatus(&hvs)
	if err != nil {
		slog.Error("get hypervisor status", "err", err)
		return
	}
	c.IndentedJSON(http.StatusOK, hvs)
}

// コールバック 仮想マシンのリスト
func (m *Marmotd) listVirtualMachines(c *gin.Context) {
	var vms []db.VirtualMachine
	err := m.dbc.GetVmsStatus(&vms)
	if err != nil {
		slog.Error("get status of virtual machines", "err", err)
		return
	}
	c.IndentedJSON(http.StatusOK, vms)
}

// コールバック VMクラスタの作成
func (m *Marmotd) createCluster(c *gin.Context) {
	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("create vm cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}

	// ハイパーバイザーの稼働チェック 結果はDBへ反映
	_, err := m.CheckHypervisors()
	if err != nil {
		slog.Error("check hypervisor status", "err", err)
		return
	}

	if err := m.CreateCluster(cnf, *etcd, *node); err != nil {
		slog.Error("create cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// コールバック VMクラスタの削除
func (m *Marmotd) destroyCluster(c *gin.Context) {
	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("prepare to delete cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	fmt.Println(cnf)

	if err := m.DestroyCluster(cnf, *etcd); err != nil {
		slog.Error("delete cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// VMの作成
func (m *Marmotd) createVm(c *gin.Context) {
	slog.Info("create vm", "etcd", *etcd)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("create vm in action", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}

	err = m.CreateVM(spec, m.NodeName)
	if err != nil {
		slog.Error("creating vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// VMの削除
func (m *Marmotd) destroyVm(c *gin.Context) {
	slog.Info("destroy vm", "etcd", *etcd)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup spec", "err", err)
		return
	}

	err = m.DestroyVM(spec, m.NodeName)
	if err != nil {
		slog.Error("delete vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// クラスタの停止
func (m *Marmotd) stopCluster(c *gin.Context) {
	slog.Info("stop cluster", "etcd", *etcd)
	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("setup Json", "err", err)
		c.JSON(400, gin.H{"msg": "Can't read JSON"})
		return
	}
	if err := m.StopCluster(cnf, *etcd); err != nil {
		slog.Error("stop cluster", "err", err)
		return
	}
}

// クラスタの再スタート
func (m *Marmotd) startCluster(c *gin.Context) {
	slog.Info("start cluster", "etcd", *etcd)
	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("setup config", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	if err := m.StartCluster(cnf, *etcd); err != nil {
		slog.Error("start cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// 仮想マシンの開始
func (m *Marmotd) startVm(c *gin.Context) {
	slog.Info("start vm", "etcd", *etcd)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup config", "err", err)
		return
	}

	err = m.StartVM(spec)
	if err != nil {
		slog.Error("start vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// 仮想マシンの停止
func (m *Marmotd) stopVm(c *gin.Context) {
	slog.Info("stop vm", "etcd", *etcd)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup config", "err", err)
		return
	}

	err = m.StopVM(spec)
	if err != nil {
		slog.Error("stop vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}
