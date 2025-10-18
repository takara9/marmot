package marmot

import (
	"fmt"
	"log/slog"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/types"
)

// クラスタ削除
func (m *Marmot) DestroyClusterInternal(cnf api.MarmotConfig) error {
	//var NotFound bool = true
	for _, spec := range *cnf.VmSpec {
		// クラスタ名とホスト名の重複チェック
		vmKey, _ := m.Db.FindByHostAndClusteName(*spec.Name, *cnf.ClusterName)
		if len(vmKey) > 0 {
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

			_, _, _, err = marmotClient.DestroyVirtualMachine(spec)
			if err != nil {
				slog.Error("", "remote request err", err)
				m.Db.UpdateVmState(vm.Key, types.ERROR) // エラー状態へ
				continue
			}
		}
	}
	return nil
}
