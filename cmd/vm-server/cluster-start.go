package main

import (
	"fmt"
	"log/slog"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/virt"
)

// クラスタ停止
func (m *Marmotd) StartCluster(cnf cf.MarmotConfig, dbUrl string) error {
	for _, spec := range cnf.VMSpec {
		vmKey, _ := m.dbc.FindByHostAndClusteName(spec.Name, cnf.ClusterName)
		if len(vmKey) == 0 {
			return errors.New("NotExistVM")
		}
		spec.Key = vmKey
		vm, err := m.dbc.GetVmByKey(vmKey)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}
		err = RemoteStartVM(vm.HvNode, spec)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}
	}
	return nil
}

func RemoteStartVM(hvNode string, spec cf.VMSpec) error {
	byteJSON, _ := json.MarshalIndent(spec, "", "    ")
	// JSON形式でポストする
	reqURL := fmt.Sprintf("http://%s:8750/%s", hvNode, "startVm")
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

// VMの開始
func (m *Marmotd) StartVM(spec cf.VMSpec) error {
	url := "qemu:///system"
	err := virt.StartVM(url, spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}
	vm, err := m.dbc.GetVmByKey(spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}

	if vm.Status == db.STOPPED {
		// ハイパーバイザーのリソースの減算と保存
		hv, err := m.dbc.GetHvByKey(vm.HvNode)
		if err != nil {
			slog.Error("", "err", err)
		}
		hv.FreeCpu = hv.FreeCpu - vm.Cpu
		hv.FreeMemory = hv.FreeMemory - vm.Memory

		err = m.dbc.PutDataEtcd(hv.Key, hv)
		if err != nil {
			slog.Error("", "err", err)
		}

		// データベースの更新
		err = m.dbc.UpdateVmState(spec.Key, db.RUNNING)
		if err != nil {
			slog.Error("", "err", err)
		}
	}
	return nil
}
