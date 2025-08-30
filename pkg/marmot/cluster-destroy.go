package marmot

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	cf "github.com/takara9/marmot/pkg/config"
)

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

// クラスタ削除
func (m *marmot) destroyCluster(cnf cf.MarmotConfig) error {
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
