package util

import (
	"fmt"
	"log/slog"
	//"os"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/virt"
	etcd "go.etcd.io/etcd/client/v3"
)

// クラスタ停止
func StopCluster(cnf cf.MarmotConfig, dbUrl string) error {
	Conn, err := db.Connect(dbUrl)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	var NotFound bool = true
	for _, spec := range cnf.VMSpec {
		vmKey, _ := db.FindByHostAndClusteName(Conn, spec.Name, cnf.ClusterName)
		if len(vmKey) > 0 {
			NotFound = false
			spec.Key = vmKey
			vm, err := db.GetVmByKey(Conn, vmKey)
			if err != nil {
				slog.Error("", "err", err)
				continue
			}
			err = RemoteStopVM(vm.HvNode, spec)
			if err != nil {
				slog.Error("", "err", err)
				continue
			}
		}
	}
	Conn.Close()

	if NotFound {
		return errors.New("NotExistVM")
	}

	return nil
}

func RemoteStopVM(hvNode string, spec cf.VMSpec) error {
	byteJSON, _ := json.MarshalIndent(spec, "", "    ")

	// JSON形式でポストする
	reqURL := fmt.Sprintf("http://%s:8750/%s", hvNode, "stopVm")
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
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 {
		return errors.New(string(body))
	}
	return nil
}

// VMの停止
func StopVM(Conn *etcd.Client, spec cf.VMSpec) error {

	// コンフィグからホスト名を取得
	vm, err := db.GetVmByKey(Conn, spec.Key)
	if err != nil {
		slog.Error("", "err", err)
		return nil
	}

	if vm.Status == db.RUNNING {
		// 仮想マシンの停止＆削除
		url := "qemu:///system"
		err = virt.StopVM(url, spec.Key)
		if err != nil {
			slog.Error("", "err", err)
		}
		// ハイパーバイザーのリソース削減保存
		hv, err := db.GetHvByKey(Conn, vm.HvNode)
		if err != nil {
			slog.Error("", "err", err)
		}
		hv.FreeCpu = hv.FreeCpu + vm.Cpu
		hv.FreeMemory = hv.FreeMemory + vm.Memory
		err = db.PutDataEtcd(Conn, hv.Key, hv)
		if err != nil {
			slog.Error("", "err", err)
		}
		// データベースの更新
		err = db.UpdateVmState(Conn, spec.Key, db.STOPPED) ////////
		if err != nil {
			slog.Error("", "err", err)
		}
	}
	return nil
}
