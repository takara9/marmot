package marmot

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	cf "github.com/takara9/marmot/pkg/config"
	db "github.com/takara9/marmot/pkg/db"
	ut "github.com/takara9/marmot/pkg/util"
)

// main から　Marmot を分離する？
type marmot struct {
	NodeName string
	EtcdUrl  string
	Db       *db.Database
}

func NewMarmot(nodeName string, etcdUrl string) (*marmot, error) {
	var m marmot
	var err error
	m.Db, err = db.NewDatabase(etcdUrl)
	if err != nil {
		return nil, err
	}
	m.NodeName = nodeName
	m.EtcdUrl = etcdUrl
	return &m, nil
}

// コールバック アクセステスト用
func (m *marmot) AccessTest(c *gin.Context) {
	// チェック機能を追加して、最終的にOK/NGを返す
	c.JSON(200, gin.H{"message": "ok"})
}

// コールバック ハイパーバイザーの状態取得
func (m *marmot) ListHypervisor(c *gin.Context) {
	// ハイパーバイザーの稼働チェック　結果はDBへ反映
	_, err := ut.CheckHypervisors(m.EtcdUrl, m.NodeName)
	if err != nil {
		slog.Error("Check if the hypervisor is up and running", "err", err)
		return
	}

	// ストレージ容量の更新 結果はDBへ反映
	err = ut.CheckHvVgAll(m.EtcdUrl, m.NodeName)
	if err != nil {
		slog.Error("Update storage capacity", "err", err)
		return
	}

	// データベースから情報を取得
	d, err := db.NewDatabase(m.EtcdUrl)
	if err != nil {
		slog.Error("connect to database", "err", err)
		return
	}
	var hvs []db.Hypervisor
	err = d.GetHvsStatus(&hvs)
	if err != nil {
		slog.Error("get hypervisor status", "err", err)
		return
	}
	c.IndentedJSON(http.StatusOK, hvs)
}

// コールバック 仮想マシンのリスト
func (m *marmot) ListVirtualMachines(c *gin.Context) {
	d, err := db.NewDatabase(m.EtcdUrl)
	if err != nil {
		slog.Error("get list virtual machines", "err", err)
		return
	}
	var vms []db.VirtualMachine
	err = d.GetVmsStatus(&vms)
	if err != nil {
		slog.Error("get status of virtual machines", "err", err)
		return
	}
	c.IndentedJSON(http.StatusOK, vms)
}

// コールバック VMクラスタの作成
func (m *marmot) CreateCluster(c *gin.Context) {
	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("create vm cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	// ハイパーバイザーの稼働チェック　結果はDBへ反映
	_, err := ut.CheckHypervisors(m.EtcdUrl, m.NodeName)
	if err != nil {
		slog.Error("check hypervisor status", "err", err)
		return
	}
	if err := m.createCluster(cnf); err != nil {
		slog.Error("create cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// コールバック VMクラスタの削除
func (m *marmot) DestroyCluster(c *gin.Context) {
	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("prepare to delete cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	if err := m.destroyCluster(cnf); err != nil {
		slog.Error("delete cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// VMの作成
func (m *marmot) CreateVm(c *gin.Context) {
	slog.Info("create vm", "etcd", m.EtcdUrl)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("create vm in action", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	err = m.createVM(spec)
	if err != nil {
		slog.Error("creating vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// VMの削除
func (m *marmot) DestroyVm(c *gin.Context) {
	slog.Info("destroy vm", "etcd", m.EtcdUrl)

	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup spec", "err", err)
		return
	}
	err = DestroyVM(m.EtcdUrl, spec, m.NodeName)
	if err != nil {
		slog.Error("delete vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// クラスタの停止
func (m *marmot) StopCluster(c *gin.Context) {
	slog.Info("stop cluster", "etcd", m.EtcdUrl)

	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("setup Json", "err", err)
		c.JSON(400, gin.H{"msg": "Can't read JSON"})
		return
	}
	if err := m.stopCluster(cnf); err != nil {
		slog.Error("stop cluster", "err", err)
		return
	}
}

// クラスタの再スタート
func (m *marmot) StartCluster(c *gin.Context) {
	slog.Info("start cluster", "etcd", m.EtcdUrl)

	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("setup config", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	if err := m.startCluster(cnf); err != nil {
		slog.Error("start cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// 仮想マシンの開始
func (m *marmot) StartVm(c *gin.Context) {
	slog.Info("start vm", "etcd", m.EtcdUrl)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup config", "err", err)
		return
	}
	err = StartVM(m.EtcdUrl, spec)
	if err != nil {
		slog.Error("start vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// 仮想マシンの停止
func (m *marmot) StopVm(c *gin.Context) {
	slog.Info("stop vm", "etcd", m.EtcdUrl)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup config", "err", err)
		return
	}
	err = StopVM(m.EtcdUrl, spec)
	if err != nil {
		slog.Error("stop vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}
