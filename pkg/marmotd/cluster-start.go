package marmotd

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/types"
)

// クラスタ開始
func (m *Marmot) StartClusterInternal(cnf api.MarmotConfig) error {
	slog.Debug("StartClusterInternal", "cnf", "")

	// リクエスト送信前にコンフィグのチェックを実施する
	if cnf.VmSpec == nil || cnf.ClusterName == nil {
		return errors.New("VM Spec or Cluster Name is not set")
	}

	for _, spec := range *cnf.VmSpec {
		vmKey, _ := m.Db.FindByHostAndClusteName(*spec.Name, *cnf.ClusterName)
		if len(vmKey) == 0 {
			return errors.New("NotExistVM")
		}
		spec.Key = &vmKey
		vm, err := m.Db.GetVmByVmKey(vmKey)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}

		hvService := fmt.Sprintf("%s:%d", *vm.HvIpAddr, *vm.HvPort)
		marmotClient, err := client.NewMarmotdEp(
			"http",
			hvService,
			"/api/v1",
			15,
		)
		if err != nil {
			continue
		}

		_, _, _, err = marmotClient.StartVirtualMachine(spec)
		if err != nil {
			slog.Error("", "remote request err", err)
			m.Db.UpdateVmStateByKey(*vm.Key, types.ERROR) // エラー状態へ
			continue
		}
	}
	return nil
}
