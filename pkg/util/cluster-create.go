package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/virt"

	etcd "go.etcd.io/etcd/client/v3"
)


// コンフィグからVMクラスタを作成する
func CreateCluster(cnf cf.MarmotConfig, dbUrl string, hvNode string) error {

	Conn, err := db.Connect(dbUrl)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// リクエスト送信前にコンフィグのチェックを実施する
	for _, spec := range cnf.VMSpec {

		// クラスタ名とホスト名の重複チェック
		vmKey, _ := db.FindByHostAndClusteName(Conn, spec.Name, cnf.ClusterName)
		if len(vmKey) > 0 {
			return fmt.Errorf("existing same name virttual machine : %v", spec.Name)
		}

		// ここに、IPアドレスの重複チェックを入れる
		if len(spec.PublicIP) > 0 {
			found, err := db.FindByPublicIPaddress(Conn, spec.PublicIP)
			if err != nil {
				return err
			}
			if found {
				return fmt.Errorf("same pubic IP address exist in the cluster IP: %v", spec.PublicIP)
			}
		}
		if len(spec.PrivateIP) > 0 {
			found, err := db.FindByPrivateIPaddress(Conn, spec.PrivateIP)
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
		vmKey, _ := db.FindByHostAndClusteName(Conn, spec.Name, cnf.ClusterName)
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
		vm.HvNode, vm.Key, vm.Uuid, err = db.AssignHvforVm(Conn, vm)
		if err != nil {
			slog.Error("", "err", err)
			break_err = true
			return_errors = err
			break
		}

		// OSのバージョン、テンプレートを設定
		fmt.Println("OSのバージョン、テンプレートを設定")
		spec.VMOsVariant = cnf.VMOsVariant
		spec.OsTempVg, spec.OsTempLv, err = db.GetOsImgTempByKey(Conn, cnf.VMOsVariant)
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

		//渡すデータはJSON形式をRESTで渡したい
		// ローカルHVでVMを作成するケースと、
		// リモートHVでVMを作成するケースが発生する。

		fmt.Println("リモートとローカル関係なしに、マイクロサービスへリクエストする")
		// リモートとローカル関係なしに、マイクロサービスへリクエストする
		db.UpdateVmState(Conn, vm.Key, db.PROVISIONING)
		err = RemoteCreateStartVM(vm.HvNode, spec)
		if err != nil {
			slog.Error("", "remote request err", err)
			break_err = true
			return_errors = err
			db.UpdateVmState(Conn, vm.Key, db.ERROR) // エラー状態へ
			break
		}
		fmt.Println("実行中へ")
		db.UpdateVmState(Conn, vm.Key, db.RUNNING) // 実行中へ

		// CoreDNS登録
		fmt.Println("DNS登録をスキップ")
		/*
			err = dns.Add(dns.DnsRecord{
				Hostname: fmt.Sprintf("%s.%s.%s", vm.Name, vm.ClusterName, "a.labo.local"),
				Ipv4:     vm.PrivateIp,
				Ttl:      60,
			}, "http://ns1.labo.local:2379")
			if err != nil {
				slog.Error("", "err", err)dns.Add()", err)
			}
		*/

	} // END OF LOOP

	if break_err {
		return return_errors
	}
	return nil
}

// リモートホストにリクエストを送信する
func RemoteCreateStartVM(hvNode string, spec cf.VMSpec) error {

	byteJSON, _ := json.MarshalIndent(spec, "", "    ")
	//fmt.Println(string(byteJSON))

	// JSON形式でポストする
	reqURL := fmt.Sprintf("http://%s:8750/%s", hvNode, "createVm")
	request, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	defer response.Body.Close()

	// レスポンスを取得する
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 {
		return errors.New(string(body))
	}
	return nil
}

// この部分は、ハイパーバイザーのホストで実行する
// VMを生成する
func CreateVM(conn *etcd.Client, spec cf.VMSpec, hvNode string) error {

	// Libvirtのテンプレートを読み込んで、設定を変更する
	var dom virt.Domain
	err := virt.ReadXml("temp.xml", &dom)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	dom.Name = spec.Key // VMを一意に識別するキーでありhostnameではない
	dom.Uuid = spec.Uuid
	dom.Vcpu.Value = spec.CPU
	var mem = spec.Memory * 1024 //KiB
	dom.Memory.Value = mem
	dom.CurrentMemory.Value = mem

	// OSボリュームを作成  (N テンプレートを指定できると良い)
	osLogicalVol, err := CreateOsLv(conn, spec.OsTempVg, spec.OsTempLv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	dom.Devices.Disk[0].Source.Dev = fmt.Sprintf("/dev/%s/%s", spec.OsTempVg, osLogicalVol)

	// OSボリュームのLV名をetcdへ登録
	err = db.UpdateOsLv(conn, spec.Key, spec.OsTempVg, osLogicalVol)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// OSボリュームをマウントして、 ホスト名、IPアドレスを設定する
	// 必要最小限として、詳細設定はAnsibleで実行する
	err = ConfigRootVol(spec, spec.OsTempVg, osLogicalVol)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// DATAボリュームを作成 (最大９個)
	dev := []string{"vdb", "vdc", "vde", "vdf", "vdg", "vdh", "vdj", "vdk", "vdl"}
	bus := []string{"0x0a", "0x0b", "0x0c", "0x0d", "0x0e", "0x0f", "0x10", "0x11", "0x12"}
	for i, disk := range spec.Storage {
		var dk virt.Disk

		// ボリュームグループが指定されていない時はvg1を指定
		var vg string = "vg1"
		if len(disk.VolGrp) > 0 {
			vg = disk.VolGrp
		}
		dlv, err := CreateDataLv(conn, uint64(disk.Size), vg)
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
		err = db.UpdateDataLv(conn, spec.Key, i, disk.VolGrp, dlv)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}
		// エラー発生時にロールバックが必要（未実装）
	}

	// ストレージの更新
	CheckHvVG2(conn, hvNode, spec.OsTempVg)

	// XMLへNICインターフェースの追加
	if len(spec.PrivateIP) > 0 {
		CreateNic("pri", &dom.Devices.Interface)
	}

	if len(spec.PublicIP) > 0 {
		CreateNic("pub", &dom.Devices.Interface)
	}

	// 仮想マシン定義のXMLファイルを生成する
	textXml := virt.CreateVirtXML(dom)
	xmlfileName := fmt.Sprintf("./%v.xml", dom.Uuid)
	file, err := os.Create(xmlfileName)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	defer file.Close()
	_, err = file.Write([]byte(textXml))
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// 仮想マシンを起動する
	url := "qemu:///system"
	err = virt.CreateStartVM(url, xmlfileName)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// 仮想マシンXMLファイルを削除する
	err = os.Remove(xmlfileName)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// 仮想マシンの状態変更(未実装)

	// 正常終了
	return nil
}
