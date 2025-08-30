package marmot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

// クラスタ削除
func DestroyCluster(cnf cf.MarmotConfig, dbUrl string) error {
	fmt.Println("DEBUG Print in DestroyCluster dburl", dbUrl)
	d, err := db.NewDatabase(dbUrl)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	var NotFound bool = true
	for _, spec := range cnf.VMSpec {
		// クラスタ名とホスト名の重複チェック
		vmKey, _ := d.FindByHostAndClusteName(spec.Name, cnf.ClusterName)
		fmt.Println("DEBUG Print in DestroyCluster vmKey, specName", vmKey, spec.Name)
		if len(vmKey) > 0 {
			NotFound = false
			spec.Key = vmKey
			vm, err := d.GetVmByKey(vmKey)
			if err != nil {
				slog.Error("", "err", err)
				continue
			}
			err = RemoteDestroyVM(vm.HvNode, spec)
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

// リモートとローカルHV上のVMを削除する
func RemoteDestroyVM(hvNode string, spec cf.VMSpec) error {
	byteJSON, _ := json.MarshalIndent(spec, "", "    ")
	fmt.Println(string(byteJSON))
	// JSON形式でポストする
	reqURL := fmt.Sprintf("http://%s:8750/%s", hvNode, "destroyVm")
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

// VMの削除
func DestroyVM(dbUrl string, spec cf.VMSpec, hvNode string) error {
	d, err := db.NewDatabase(dbUrl)
	if err != nil {
		slog.Error("get list virtual machines", "err", err)
		return err
	}
	vm, err := d.GetVmByKey(spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}

	// ハイパーバイザーのリソース削減保存のため値を取得
	hv, err := d.GetHvByKey(vm.HvNode)
	if err != nil {
		slog.Error("", "err", err)
	}

	// ステータスを調べて停止中であれば、足し算しない。
	if vm.Status != db.STOPPED || vm.Status == db.ERROR {
		hv.FreeCpu = hv.FreeCpu + vm.Cpu
		hv.FreeMemory = hv.FreeMemory + vm.Memory
		err = d.PutDataEtcd(hv.Key, hv)
		if err != nil {
			slog.Error("", "err", err)
		}
	}

	// データベースから削除
	err = d.DelByKey(spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}

	// 仮想マシンの停止＆削除
	url := "qemu:///system"
	err = virt.DestroyVM(url, spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}

	// OS LVを削除
	err = lvm.RemoveLV(vm.OsVg, vm.OsLv)
	if err != nil {
		slog.Error("", "err", err)
	}
	// ストレージの更新
	util.CheckHvVG2(dbUrl, hvNode, vm.OsVg)

	// データLVを削除
	for _, dd := range vm.Storage {
		err = lvm.RemoveLV(dd.Vg, dd.Lv)
		if err != nil {
			slog.Error("", "err", err)
		}
		// ストレージの更新
		util.CheckHvVG2(dbUrl, hvNode, dd.Vg)
	}

	return nil
}
