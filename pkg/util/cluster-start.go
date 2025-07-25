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
func StartCluster(cnf cf.MarmotConfig, dbUrl string) error {
	Conn, err := db.Connect(dbUrl)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	for _, spec := range cnf.VMSpec {

		vmKey, _ := db.FindByHostAndClusteName(Conn, spec.Name, cnf.ClusterName)
		if len(vmKey) == 0 {
			return errors.New("NotExistVM")
		}
		spec.Key = vmKey
		vm, err := db.GetVmByKey(Conn, vmKey)
		if err != nil {
			slog.Error("", "err", err)
			Conn.Close()
			return err
		}
		err = RemoteStartVM(vm.HvNode, spec)
		if err != nil {
			slog.Error("", "err", err)
			Conn.Close()
			return err
		}
	}
	Conn.Close()
	return nil
}

func RemoteStartVM(hvNode string, spec cf.VMSpec) error {
	byteJSON, _ := json.MarshalIndent(spec, "", "    ")
	//fmt.Println(string(byteJSON))

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
func StartVM(Conn *etcd.Client, spec cf.VMSpec) error {

	// 仮想マシンの開始
	url := "qemu:///system"
	err := virt.StartVM(url, spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}
	vm, err := db.GetVmByKey(Conn, spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}

	if vm.Status == db.STOPPED {

		// ハイパーバイザーのリソースの減算と保存
		hv, err := db.GetHvByKey(Conn, vm.HvNode)
		if err != nil {
			slog.Error("", "err", err)
		}
		hv.FreeCpu = hv.FreeCpu - vm.Cpu
		hv.FreeMemory = hv.FreeMemory - vm.Memory

		err = db.PutDataEtcd(Conn, hv.Key, hv)
		if err != nil {
			slog.Error("", "err", err)
		}

		// データベースの更新
		err = db.UpdateVmState(Conn, spec.Key, db.RUNNING)
		if err != nil {
			slog.Error("", "err", err)
		}
	}
	return nil
}
