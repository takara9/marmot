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

// クラスタの再スタート
func (m *Marmot) StartCluster(c *gin.Context) {
	slog.Info("start cluster", "etcd", m.EtcdUrl)

	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("setup config", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	if err := m.StartCluster2(cnf); err != nil {
		slog.Error("start cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// クラスタ停止
func (m *Marmot) StartCluster2(cnf cf.MarmotConfig) error {
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

// クラスタ開始
func (m *Marmot) StartClusterInternal(cnf api.MarmotConfig) error {
	for _, spec := range *cnf.VmSpec {
		vmKey, _ := m.Db.FindByHostAndClusteName(*spec.Name, *cnf.ClusterName)
		if len(vmKey) == 0 {
			return errors.New("NotExistVM")
		}
		spec.Key = &vmKey
		vm, err := m.Db.GetVmByKey(vmKey)
		if err != nil {
			slog.Error("", "err", err)
			return err
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

		_, _, _, err = marmotClient.StartVirtualMachine(vm.HvNode, spec)
		if err != nil {
			slog.Error("", "remote request err", err)
			m.Db.UpdateVmState(vm.Key, db.ERROR) // エラー状態へ
			continue
		}
	}
	return nil
}
