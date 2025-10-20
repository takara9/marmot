package marmot

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/types"
)

// コンフィグからVMクラスタを作成する  新APIを使用
func (m *Marmot) CreateClusterInternal(cnf api.MarmotConfig) error {
	var err error
	// リクエスト送信前にコンフィグのチェックを実施する
	for _, spec := range *cnf.VmSpec {
		// クラスタ名とホスト名の重複チェック
		vmKey, _ := m.Db.FindByHostAndClusteName(*spec.Name, *cnf.ClusterName)
		if len(vmKey) > 0 {
			return fmt.Errorf("existing same name virttual machine : %v", spec.Name)
		}
		// ここに、IPアドレスの重複チェックを入れる
		if len(*spec.PublicIp) > 0 {
			found, err := m.Db.FindByPublicIPaddress(*spec.PublicIp)
			if err != nil {
				return err
			}
			if found {
				return fmt.Errorf("same pubic IP address exist in the cluster IP: %v", *spec.PublicIp)
			}
		}
		if len(*spec.PrivateIp) > 0 {
			found, err := m.Db.FindByPrivateIPaddress(*spec.PrivateIp)
			if err != nil {
				return err
			}
			if found {
				return fmt.Errorf("same private IP address exist in the cluster IP: %v", *spec.PrivateIp)
			}
		}
	} // END OF LOOP
	var break_err bool = false
	return_errors := errors.New("")
	// 仮想マシンの設定と起動
	for _, spec := range *cnf.VmSpec {
		// ホスト名とクラスタ名でVMキーを取得する
		vmKey, _ := m.Db.FindByHostAndClusteName(*spec.Name, *cnf.ClusterName)
		if len(vmKey) > 0 {
			continue
		}
		// 新API Config, VmSpec から DBの構造体へセット
		vm := convApiConfigToDB(spec, cnf)

		//スケジュールを実行
		vm.HvNode, vm.HvIpAddr, vm.Key, vm.Uuid, vm.HvPort, err = m.Db.AssignHvforVm(vm)
		if err != nil {
			slog.Error("", "err", err)
			break_err = true
			return_errors = err
			break
		}

		// OSのバージョン、テンプレートを設定
		fmt.Println("OSのバージョン、テンプレートを設定")
		spec.Ostempvariant = cnf.OsVariant
		vg, lv, err := m.Db.GetOsImgTempByKey(*spec.Ostempvariant)
		spec.Ostempvg = &vg
		spec.Ostemplv = &lv
		if err != nil {
			slog.Error("", "err", err)
			break_err = true
			return_errors = err
			break
		}

		// VMのUUIDとKEYをコンフィグ情報へセット
		fmt.Println("VMのUUIDとKEYをコンフィグ情報へセット")
		u := vm.Uuid.String()
		spec.Uuid = &u // こんな方法は正しいのか？
		k := vm.Key
		spec.Key = &k

		// 問題発見処理
		if len(vm.HvNode) == 0 {
			break_err = true
			return_errors = err
			break
		}
		if len(vm.Name) == 0 {
			break_err = true
			return_errors = err
			break
		}

		// リモートとローカル関係なしに、マイクロサービスへリクエストする
		m.Db.UpdateVmState(vm.Key, types.PROVISIONING)

		marmotHost := fmt.Sprintf("%s:%d", vm.HvIpAddr, vm.HvPort)
		marmotClient, err := NewMarmotdEp(
			"http",
			marmotHost,
			"/api/v1",
			15,
		)
		if err != nil {
			continue
		}
		_, _, _, err = marmotClient.CreateVirtualMachine(spec)
		if err != nil {
			slog.Error("", "remote request err", err)
			break_err = true
			return_errors = err
			m.Db.UpdateVmState(vm.Key, types.ERROR) // エラー状態へ
			break
		}
		fmt.Println("実行中へ")
		m.Db.UpdateVmState(vm.Key, types.RUNNING) // 実行中へ

		fmt.Println("DNS登録をスキップ")
	} // END OF LOOP

	if break_err {
		return return_errors
	}
	return nil
}

// HVへVMスケジュールするために db.VirtualMachineにセットする
func convApiConfigToDB(spec api.VmSpec, cnf api.MarmotConfig) types.VirtualMachine {
	var vm types.VirtualMachine
	vm.ClusterName = *cnf.ClusterName
	vm.OsVariant = *cnf.OsVariant
	vm.Name = *spec.Name // Os のhostname
	vm.Cpu = int(*spec.Cpu)
	vm.Memory = int(*spec.Memory)
	vm.PrivateIp = *spec.PrivateIp
	vm.PublicIp = *spec.PublicIp
	vm.Playbook = *spec.Playbook
	vm.Comment = *spec.Comment
	vm.Status = types.INITALIZING
	for _, stg := range *spec.Storage {
		var vms types.Storage
		vms.Name = *stg.Name
		vms.Size = int(*stg.Size)
		vms.Path = *stg.Path
		vm.Storage = append(vm.Storage, vms)
	}
	return vm
}
