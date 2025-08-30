package marmot

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"
	cf "github.com/takara9/marmot/pkg/config"
)

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

// クラスタ停止
func (m *marmot) startCluster(cnf cf.MarmotConfig) error {
	for _, spec := range cnf.VMSpec {
		vmKey, _ := m.Db.FindByHostAndClusteName(spec.Name, cnf.ClusterName)
		if len(vmKey) == 0 {
			return errors.New("NotExistVM")
		}
		spec.Key = vmKey
		vm, err := m.Db.GetVmByKey(vmKey)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}
		err = startRemoteVM(vm.HvNode, spec)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}
	}
	return nil
}
