package marmot

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"
	cf "github.com/takara9/marmot/pkg/config"
)

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

// クラスタ停止
func (m *marmot) stopCluster(cnf cf.MarmotConfig) error {
	var NotFound bool = true
	for _, spec := range cnf.VMSpec {
		vmKey, _ := m.Db.FindByHostAndClusteName(spec.Name, cnf.ClusterName)
		if len(vmKey) > 0 {
			NotFound = false
			spec.Key = vmKey
			vm, err := m.Db.GetVmByKey(vmKey)
			if err != nil {
				slog.Error("", "err", err)
				continue
			}
			err = stopRemoteVM(vm.HvNode, spec)
			if err != nil {
				slog.Error("", "err", err)
				continue
			}
		}
	}
	if NotFound {
		return errors.New("NotExistVM")
	}
	return nil
}
