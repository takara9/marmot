package marmot

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	db "github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/types"
	ut "github.com/takara9/marmot/pkg/util"
)

// main から　Marmot を分離する？
type Marmot struct {
	NodeName string
	EtcdUrl  string
	Db       *db.Database
}

func NewMarmot(nodeName string, etcdUrl string) (*Marmot, error) {
	var m Marmot
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
//func (m *Marmot) AccessTest(c *gin.Context) {
// チェック機能を追加して、最終的にOK/NGを返す
//	c.JSON(200, gin.H{"message": "ok"})
//}

// コールバック ハイパーバイザーの状態取得
func (m *Marmot) ListHypervisor(c *gin.Context) {
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
	var hvs []types.Hypervisor
	err = d.GetHypervisors(&hvs)
	if err != nil {
		slog.Error("get hypervisor status", "err", err)
		return
	}
	c.IndentedJSON(http.StatusOK, hvs)
}

// コールバック 仮想マシンのリスト
func (m *Marmot) ListVirtualMachines(c *gin.Context) {
	d, err := db.NewDatabase(m.EtcdUrl)
	if err != nil {
		slog.Error("get list virtual machines", "err", err)
		return
	}
	var vms []types.VirtualMachine
	err = d.GetVmsStatus(&vms)
	if err != nil {
		slog.Error("get status of virtual machines", "err", err)
		return
	}
	c.IndentedJSON(http.StatusOK, vms)
}
