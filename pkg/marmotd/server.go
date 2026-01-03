package marmotd

import (
	"fmt"
	"log/slog"

	"github.com/takara9/marmot/api"
)

// 仮想マシンの生成、qcow2に対応すること、仮想マシンを識別するIDは、ホスト名ではなくUUIDであることに注意
// volume の生成は、volumes.goに任せること！
func (m *Marmot) CreateServer(spec api.Server) (string, error) {
	slog.Debug("=====CreateServer()=====", "spec", spec)

	slog.Debug("仮想マシンのIDを付与")
	server, err := m.Db.CreateServer(spec)
	if err != nil {
		slog.Error("CreateServer()", "err", err)
		return "", err
	}

	fmt.Println("New Server ID:", server.Id)

	slog.Debug("仮想マシンの定義を取得")

	slog.Debug("ハイパーバイザーのリソース確保")

	slog.Debug("etcdのキーを設定")

	slog.Debug("CPU数設定がなければ、CPU数のデフォルトを設定")

	slog.Debug("メモリサイズのメモリサイズのデフォルトを設定")

	slog.Debug("OS指定がなければ、OSバリアントのデフォルトを設定")

	slog.Debug("ボリュームタイプの指定がなければ、qcow2を設定")

	slog.Debug("ネットワークの設定")

	slog.Debug("データボリュームの生成と設定")

	slog.Debug("libvirtのXML定義の生成")

	slog.Debug("仮想マシンの起動")

	slog.Debug("データベースに登録")
	//svc, err := m.Db.CreateServer(spec)
	//if err != nil {
	//	slog.Error("CreateServer()", "err", err)
	//	//return "", err
	//}
	slog.Debug("CreateServer()", "id", server.Id)
	/*
		// 仮想マシンの定義を取得
		var dom virt.Domain
		err := virt.ReadXml("temp.xml", &dom)
		if err != nil {
			slog.Error("virt.ReadXml()", "err", err)
			return err
		}

		// どこでシリアルをつけているのか、問題だ
		if spec.Key != nil {
			//dom.Name = *spec.Key // VMを一意に識別するキーでありhostnameではない
			x := strings.Split(*spec.Key, "/")
			dom.Name = x[len(x)-1] // VMを一意に識別するキーでありhostnameではない
		}

		if spec.Uuid != nil {
			dom.Uuid = *spec.Uuid
		}
		if spec.Cpu != nil {
			dom.Vcpu.Value = int(*spec.Cpu)
		}
		if spec.Memory != nil {
			var mem = int(*spec.Memory) * 1024 //KiB
			dom.Memory.Value = mem
			dom.CurrentMemory.Value = mem
		}

		slog.Debug("CreateOsLv()", "spec", "check")
		if spec.Ostempvg == nil || spec.Ostemplv == nil {
			slog.Error("OS Temp VG or OS Temp LV is null", "", "")
			return fmt.Errorf("OS Temp VG or OS Temp LV is null")
		}

		osLogicalVol, err := m.Db.CreateOsLv(*spec.Ostempvg, *spec.Ostemplv)
		if err != nil {
			slog.Error("util.CreateOsLv()", "err", err)
			return err
		}
		slog.Debug("CreateOsLv()", "lv", osLogicalVol)

		err = m.Db.UpdateOsLvByVmKey(*spec.Key, *spec.Ostempvg, osLogicalVol)
		if err != nil {
			slog.Error("util.CreateOsLv()", "err", err, "*spec.Key", *spec.Key)
			return err
		}

		dom.Devices.Disk[0].Source.Dev = fmt.Sprintf("/dev/%s/%s", *spec.Ostempvg, osLogicalVol)

		if err := util.ConfigRootVol2(spec, *spec.Ostempvg, osLogicalVol); err != nil {
			slog.Error("util.CreateOsLv()", "err", err)
			return err
		}

		slog.Debug("spec.Storage", "start", "")

		if spec.Storage != nil {
			// DATAボリュームを作成 (最大９個)
			dev := []string{"vdb", "vdc", "vde", "vdf", "vdg", "vdh", "vdj", "vdk", "vdl"}
			bus := []string{"0x0a", "0x0b", "0x0c", "0x0d", "0x0e", "0x0f", "0x10", "0x11", "0x12"}
			for i, disk := range *spec.Storage {
				if disk.Size == nil {
					continue
				}
				slog.Debug("spec.Storage", "disk size", *disk.Size)
				var dk virt.Disk
				// ボリュームグループが指定されていない時はvg1を指定
				var vg string = "vg1"
				if disk.Vg != nil {
					vg = *disk.Vg
				}
				dlv, err := m.Db.CreateDataLv(uint64(*disk.Size), vg)
				if err != nil {
					slog.Error("", "err", err)
					return err
				}
				// LibVirtの設定を追加
				dk.Type = "block"
				dk.Device = "disk"
				dk.Driver.Name = "qemu"
				dk.Driver.Type = "raw"
				dk.Driver.Cache = "none"
				dk.Driver.Io = "native"
				dk.Source.Dev = fmt.Sprintf("/dev/%s/%s", vg, dlv)
				dk.Target.Dev = dev[i]
				dk.Target.Bus = "virtio"
				dk.Address.Type = "pci"
				dk.Address.Domain = "0x0000"
				dk.Address.Bus = bus[i]
				dk.Address.Slot = "0x00"
				dk.Address.Function = "0x0"
				// 配列に追加
				dom.Devices.Disk = append(dom.Devices.Disk, dk)
				// etcdデータベースにlvを登録
				err = m.Db.UpdateDataLvByVmKey(*spec.Key, i, *disk.Vg, dlv)
				if err != nil {
					slog.Error("", "err", err)
					return err
				}
				// エラー発生時にロールバックが必要（未実装）
			}
			// ストレージの更新
			m.Db.CheckHvVG2ByName(m.NodeName, *spec.Ostempvg)
		}
		slog.Debug("spec.Storage", "finish", "")

		if spec.PrivateIp != nil {
			util.CreateNic("pri", &dom.Devices.Interface)
		}

		if spec.PublicIp != nil {
			util.CreateNic("pub", &dom.Devices.Interface)
		}

		textXml := virt.CreateVirtXML(dom)
		xmlfileName := fmt.Sprintf("./%v.xml", dom.Uuid)
		file, err := os.Create(xmlfileName)
		if err != nil {
			slog.Error("os.Create()", "err", err)
			return err
		}
		defer file.Close()

		_, err = file.Write([]byte(textXml))
		if err != nil {
			slog.Error("file.Write()", "err", err)
			return err
		}

		url := "qemu:///system"
		err = virt.CreateStartVM(url, xmlfileName)
		if err != nil {
			slog.Error("virt.CreateStartVM()", "err", err)
			return err
		}

		// 仮想マシンXMLファイルを削除する
		err = os.Remove(xmlfileName)
		if err != nil {
			slog.Error("os.Remove()", "err", err)
			return err
		}

	*/
	// 仮想マシンの状態変更(未実装)

	return server.Id, nil
}

// 仮想マシンの削除、qcow2に対応すること、仮想マシンを識別するIDは、ホスト名ではなくUUIDであることに注意
// volume の生成は、volumes.goに任せること！
func (m *Marmot) DeleteServerById(id string) error {
	slog.Debug("===", "DeleteServerById is called", "===")
	err := m.Db.DeleteServerById(id)
	if err != nil {
		slog.Error("DeleteServerById()", "err", err)
		return err
	}
	/*
		if spec.Key != nil {
			slog.Debug("DestroyServer()", "key", *spec.Key)
		}

		var vm api.VirtualMachine
		var err error

		if spec.Key != nil {
			vm, err = m.Db.GetVmByVmKey(*spec.Key)
			if err != nil {
				slog.Error("GetVmByVmKey()", "err", err)
			}
		}

		// ハイパーバイザーのリソース削減保存のため値を取得
		hv, err := m.Db.GetHypervisorByName(vm.HvNode)
		if err != nil {
			slog.Error("GetHypervisorByName()", "err", err)
		}

		// ステータスを調べて停止中であれば、足し算しない。
		if *vm.Status != types.STOPPED && *vm.Status != types.ERROR {
			*hv.FreeCpu = *hv.FreeCpu + int32(*vm.Cpu)
			*hv.FreeMemory = *hv.FreeMemory + *vm.Memory
			err = m.Db.PutJSON(*hv.Key, hv)
			if err != nil {
				slog.Error("PutDataEtcd()", "err", err)
			}
		}

		slog.Debug("DestroyVM2() proceed to delete VM on database", "vmKey", *spec.Key)
		// データベースからVMを削除
		if err := m.Db.DeleteJSON(*spec.Key); err != nil {
			slog.Error("DeleteJSON(", "err", err)
		}

		// 仮想マシンの停止＆削除
		domName := strings.Split(*spec.Key, "/")
		slog.Debug("DestroyVM2() proceed to delete VM on hypervisor", "vmKey", *spec.Key, "domName", domName[len(domName)-1])

		if err := virt.DestroyVM("qemu:///system", domName[len(domName)-1]); err != nil {
			slog.Error("DestroyVM()", "err", err, "vmKey", *spec.Key, "key", domName[len(domName)-1])
		}

		// OS LVを削除
		slog.Debug("DestroyVM2() proceed to delete OS LV", "vm.OsVg", *vm.OsVg, "vm.OsLv", *vm.OsLv)
		if err := lvm.RemoveLV(*vm.OsVg, *vm.OsLv); err != nil {
			slog.Error("lvm.RemoveLV()", "err", err)
		}

		// ストレージの更新
		m.Db.CheckHvVG2ByName(m.NodeName, *vm.OsVg)

		// データLVを削除
		if vm.Storage != nil {
			for _, dd := range *vm.Storage {
				slog.Debug("DestroyVM2() proceed to delete Data LV", "dd.Vg", *dd.Vg, "dd.Lv", *dd.Lv)
				err = lvm.RemoveLV(*dd.Vg, *dd.Lv)
				if err != nil {
					slog.Error("RemoveLV()", "err", err)
				}
				// ストレージの更新
				m.Db.CheckHvVG2ByName(m.NodeName, *dd.Vg)
			}
		}
	*/
	return nil
}

// サーバーのリストを取得、フィルターは、パラメータで指定するようにする
func (m *Marmot) GetServers() (api.Servers, error) {
	slog.Debug("===", "GetServers is called", "===")
	svc, err := m.Db.GetServers()
	if err != nil {
		slog.Error("GetServers()", "err", err)
		return nil, err
	}
	slog.Debug("GetServers()", "svc", svc)
	return svc, nil
}

// サーバーの詳細を取得
func (m *Marmot) GetServerById(id string) (api.Server, error) {
	slog.Debug("===", "GetServerById is called", "===")
	server, err := m.Db.GetServerById(id)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return api.Server{}, err
	}
	slog.Debug("GetServerById()", "svc", server)

	return server, nil
}

// サーバーの更新
func (m *Marmot) UpdateServerById(id string, spec api.Server) error {
	slog.Debug("===", "UpdateServerById is called", "===")
	err := m.Db.UpdateServer(id, spec)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return err
	}
	slog.Debug("UpdateServerById()", "svc", nil)
	return nil
}
