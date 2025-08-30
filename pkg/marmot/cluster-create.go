package marmot

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	ut "github.com/takara9/marmot/pkg/util"
)

// コールバック VMクラスタの作成
func (m *marmot) CreateCluster(c *gin.Context) {
	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("create vm cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	// ハイパーバイザーの稼働チェック　結果はDBへ反映
	_, err := ut.CheckHypervisors(m.EtcdUrl, m.NodeName)
	if err != nil {
		slog.Error("check hypervisor status", "err", err)
		return
	}
	if err := m.createCluster(cnf); err != nil {
		slog.Error("create cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// コンフィグからVMクラスタを作成する
func (m *marmot) createCluster(cnf cf.MarmotConfig) error {
	var err error
	// リクエスト送信前にコンフィグのチェックを実施する
	for _, spec := range cnf.VMSpec {
		// クラスタ名とホスト名の重複チェック
		vmKey, _ := m.Db.FindByHostAndClusteName(spec.Name, cnf.ClusterName)
		if len(vmKey) > 0 {
			return fmt.Errorf("existing same name virttual machine : %v", spec.Name)
		}
		// ここに、IPアドレスの重複チェックを入れる
		if len(spec.PublicIP) > 0 {
			found, err := m.Db.FindByPublicIPaddress(spec.PublicIP)
			if err != nil {
				return err
			}
			if found {
				return fmt.Errorf("same pubic IP address exist in the cluster IP: %v", spec.PublicIP)
			}
		}
		if len(spec.PrivateIP) > 0 {
			found, err := m.Db.FindByPrivateIPaddress(spec.PrivateIP)
			if err != nil {
				return err
			}
			if found {
				return fmt.Errorf("same private IP address exist in the cluster IP: %v", spec.PrivateIP)
			}
		}
	} // END OF LOOP

	var break_err bool = false
	return_errors := errors.New("")
	// 仮想マシンの設定と起動
	for _, spec := range cnf.VMSpec {
		fmt.Println("ホスト名とクラスタ名でVMキーを取得する")
		// ホスト名とクラスタ名でVMキーを取得する
		vmKey, _ := m.Db.FindByHostAndClusteName(spec.Name, cnf.ClusterName)
		if len(vmKey) > 0 {
			continue
		}

		// HVへVMスケジュールするために db.VirtualMachineにセットする
		var vm db.VirtualMachine
		vm.ClusterName = cnf.ClusterName
		vm.OsVariant = cnf.VMOsVariant
		vm.Name = spec.Name // Os のhostname
		vm.Cpu = spec.CPU
		vm.Memory = spec.Memory
		vm.PrivateIp = spec.PrivateIP
		vm.PublicIp = spec.PublicIP
		vm.Playbook = spec.AnsiblePB
		vm.Comment = spec.Comment
		vm.Status = db.INITALIZING
		for _, stg := range spec.Storage {
			var vms db.Storage
			vms.Name = stg.Name
			vms.Size = stg.Size
			vms.Path = stg.Path
			vm.Storage = append(vm.Storage, vms)
		}

		//スケジュールを実行
		fmt.Println("スケジュールを実行")
		vm.HvNode, vm.Key, vm.Uuid, err = m.Db.AssignHvforVm(vm)
		if err != nil {
			slog.Error("", "err", err)
			break_err = true
			return_errors = err
			break
		}

		// OSのバージョン、テンプレートを設定
		fmt.Println("OSのバージョン、テンプレートを設定")
		spec.VMOsVariant = cnf.VMOsVariant
		spec.OsTempVg, spec.OsTempLv, err = m.Db.GetOsImgTempByKey(cnf.VMOsVariant)
		if err != nil {
			slog.Error("", "err", err)
			break_err = true
			return_errors = err
			break
		}

		// VMのUUIDとKEYをコンフィグ情報へセット
		fmt.Println("VMのUUIDとKEYをコンフィグ情報へセット")
		spec.Uuid = vm.Uuid.String()
		spec.Key = vm.Key

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

		fmt.Println("リモートとローカル関係なしに、マイクロサービスへリクエストする")
		// リモートとローカル関係なしに、マイクロサービスへリクエストする
		m.Db.UpdateVmState(vm.Key, db.PROVISIONING)
		err = createRemoteVM(vm.HvNode, spec)
		if err != nil {
			slog.Error("", "remote request err", err)
			break_err = true
			return_errors = err
			m.Db.UpdateVmState(vm.Key, db.ERROR) // エラー状態へ
			break
		}
		fmt.Println("実行中へ")
		m.Db.UpdateVmState(vm.Key, db.RUNNING) // 実行中へ

		fmt.Println("DNS登録をスキップ")
	} // END OF LOOP

	if break_err {
		return return_errors
	}
	return nil
}
