package marmot

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/takara9/marmot/api"
	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
)

// クラスタの停止
func (m *Marmot) StopCluster(c *gin.Context) {
	slog.Info("stop cluster", "etcd", m.EtcdUrl)

	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("setup Json", "err", err)
		c.JSON(400, gin.H{"msg": "Can't read JSON"})
		return
	}
	if err := m.StopCluster2(cnf); err != nil {
		slog.Error("stop cluster", "err", err)
		return
	}
}

// クラスタ停止
func (m *Marmot) StopCluster2(cnf cf.MarmotConfig) error {
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

// クラスタ停止
func (m *Marmot) StopClusterInternal(cnf api.MarmotConfig) error {
	var NotFound bool = true
	for _, spec := range *cnf.VmSpec {
		vmKey, _ := m.Db.FindByHostAndClusteName(*spec.Name, *cnf.ClusterName)
		if len(vmKey) > 0 {
			NotFound = false
			spec.Key = &vmKey
			vm, err := m.Db.GetVmByKey(vmKey)
			if err != nil {
				slog.Error("", "err", err)
				continue
			}

			hvService := fmt.Sprintf("%s:%d", vm.HvNode, vm.HvPort)
			marmotClient, err := NewMarmotdEp(
				"http",
				hvService,
				"/api/v1",
				15,
			)
			if err != nil {
				continue
			}
			_, _, _, err = marmotClient.StopVirtualMachine(spec)
			if err != nil {
				slog.Error("", "remote request err", err)
				m.Db.UpdateVmState(vm.Key, db.ERROR) // エラー状態へ
				continue
			}
		}
	}
	if NotFound {
		return errors.New("NotExistVM")
	}
	return nil
}
