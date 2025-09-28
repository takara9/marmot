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

// コールバック VMクラスタの削除
func (m *Marmot) DestroyCluster(c *gin.Context) {
	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("prepare to delete cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	if err := m.DestroyCluster2(cnf); err != nil {
		slog.Error("delete cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// クラスタ削除
func (m *Marmot) DestroyCluster2(cnf cf.MarmotConfig) error {
	var NotFound bool = true
	for _, spec := range cnf.VMSpec {
		// クラスタ名とホスト名の重複チェック
		vmKey, _ := m.Db.FindByHostAndClusteName(spec.Name, cnf.ClusterName)
		fmt.Println("DEBUG Print in DestroyCluster vmKey, specName", vmKey, spec.Name)
		if len(vmKey) > 0 {
			NotFound = false
			spec.Key = vmKey
			vm, err := m.Db.GetVmByKey(vmKey)
			if err != nil {
				slog.Error("", "err", err)
				continue
			}
			err = destroyRemoteVM(vm.HvNode, spec)
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

// クラスタ削除
func (m *Marmot) DestroyClusterInternal(cnf api.MarmotConfig) error {
	//var NotFound bool = true
	for _, spec := range *cnf.VmSpec {
		// クラスタ名とホスト名の重複チェック
		vmKey, _ := m.Db.FindByHostAndClusteName(*spec.Name, *cnf.ClusterName)
		fmt.Println("DEBUG Print in DestroyCluster vmKey=", vmKey, "specName=", *spec.Name)
		if len(vmKey) > 0 {
			//NotFound = false
			spec.Key = &vmKey

			vm, err := m.Db.GetVmByKey(vmKey)
			if err != nil {
				slog.Error("", "err", err)
				continue
			}

			//marmotClient, err := NewMarmotdEp(
			//	"http",
			//	"localhost:8080",
			//	"/api/v1",
			//	60,
			//)
			marmotClient, err := NewMarmotdEp(
				"http",
				vm.HvNode,
				"/api/v1",
				15,
			)
			if err != nil {
				continue
			}

			_, _, _, err = marmotClient.DestroyVirtualMachine(vm.HvNode, spec)
			if err != nil {
				slog.Error("", "remote request err", err)
				m.Db.UpdateVmState(vm.Key, db.ERROR) // エラー状態へ
				continue
			}
		}
	}
	//if NotFound {
	//	return errors.New("NotExistVM")
	//}
	return nil
}
