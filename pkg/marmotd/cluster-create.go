package marmotd

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/types"
)

// コンフィグからVMクラスタを作成する  新APIを使用
func (m *Marmot) CreateClusterInternal(cnf api.MarmotConfig) error {
	if DEBUG {
		printConfigJson(cnf)
	}
	slog.Debug("CreateClusterInternal", "cnf", "")

	// リクエスト送信前にコンフィグのチェックを実施する
	if cnf.ClusterName == nil {
		slog.Debug("cnf.ClusterName", "val", "is not set")
		return errors.New("cluster name is not set")
	}
	if cnf.VmSpec == nil {
		return errors.New("vm spec is not set")
	}
	if cnf.OsVariant == nil {
		return errors.New("OS template is not set")
	}

	for _, spec := range *cnf.VmSpec {
		// クラスタ名とホスト名の重複チェック
		if spec.Name == nil {
			return errors.New("VM Name is not set")
		}
		vmKey, _ := m.Db.FindByHostAndClusteName(*spec.Name, *cnf.ClusterName)
		if len(vmKey) > 0 {
			return fmt.Errorf("existing same name virttual machine : %v", spec.Name)
		}

		// パブリックIPアドレスの重複チェックを入れる
		if spec.PublicIp != nil {
			found, err := m.Db.FindByPublicIPaddress(*spec.PublicIp)
			if err != nil {
				return err
			}
			if found {
				return fmt.Errorf("same pubic IP address exist in the cluster IP: %v", *spec.PublicIp)
			}
		}

		// プライベートIPアドレスの重複チェックを入れる
		if spec.PrivateIp != nil {
			found, err := m.Db.FindByPrivateIPaddress(*spec.PrivateIp)
			if err != nil {
				return err
			}
			if found {
				return fmt.Errorf("same private IP address exist in the cluster IP: %v", *spec.PrivateIp)
			}
		}
	} // END OF LOOP

	// 仮想マシンの設定と起動
	var break_err bool = false
	var return_err error = nil

	for _, spec := range *cnf.VmSpec {
		// ホスト名とクラスタ名でVMキーを取得する
		vmKey, _ := m.Db.FindByHostAndClusteName(*spec.Name, *cnf.ClusterName)
		if len(vmKey) > 0 {
			continue
		}
		// 新API Config, VmSpec から DBの構造体へセット
		vm := convApiConfigToDB(spec, cnf)

		//スケジュールを実行
		var err error
		vm.HvNode, vm.HvIpAddr, vm.Key, vm.Uuid, vm.HvPort, err = m.Db.AssignHvforVm(vm)
		if err != nil {
			slog.Error("", "err", err)
			break_err = true
			return_err = err
			break
		}

		// OSのバージョン、テンプレートを設定
		spec.Ostempvariant = cnf.OsVariant
		vg, lv, err := m.Db.GetOsImgTempByKey(*spec.Ostempvariant)
		spec.Ostempvg = &vg
		spec.Ostemplv = &lv
		if err != nil {
			slog.Error("", "err", err)
			break_err = true
			return_err = err
			break
		}

		// VMのUUIDとKEYをコンフィグ情報へセット
		u := vm.Uuid.String()
		spec.Uuid = &u // こんな方法は正しいのか？
		k := vm.Key
		spec.Key = &k

		// 問題発見処理
		if len(vm.HvNode) == 0 {
			break_err = true
			return_err = err
			break
		}
		if len(vm.Name) == 0 {
			break_err = true
			return_err = err
			break
		}

		// リモートとローカル関係なしに、マイクロサービスへリクエストする
		m.Db.UpdateVmState(vm.Key, types.PROVISIONING)
		marmotHost := fmt.Sprintf("%s:%d", vm.HvIpAddr, vm.HvPort)
		marmotClient, err := client.NewMarmotdEp(
			"http",
			marmotHost,
			"/api/v1",
			15,
		)
		// エラーが発生しても、次のVMの作成を続ける
		if err != nil {
			continue
		}
		_, _, _, err = marmotClient.CreateVirtualMachine(spec)
		if err != nil {
			slog.Error("", "remote request err", err)
			break_err = true
			return_err = err
			m.Db.UpdateVmState(vm.Key, types.ERROR) // エラー状態へ
			break
		}
		m.Db.UpdateVmState(vm.Key, types.RUNNING) // 実行中へ
	} // END OF LOOP

	if break_err {
		return return_err
	}
	slog.Debug("CreateClusterInternal()", "return_err", return_err)
	slog.Debug("CreateClusterInternal()", "break_err", break_err)

	return nil
}
