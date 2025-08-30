package marmot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/virt"
)

// クラスタの再スタート
func (m *marmot) StartCluster(c *gin.Context) {
	slog.Info("start cluster", "etcd", m.EtcdUrl)

	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("setup config", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	if err := m.startCluster(cnf); err != nil {
		slog.Error("start cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// クラスタ停止
func (m *marmot) startCluster(cnf cf.MarmotConfig) error {
	for _, spec := range cnf.VMSpec {
		vmKey, _ := m.Db.FindByHostAndClusteName(spec.Name, cnf.ClusterName)
		if len(vmKey) == 0 {
			return errors.New("NotExistVM")
		}
		spec.Key = vmKey
		vm, err := m.Db.GetVmByKey(vmKey)
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

// 仮想マシンの開始
func (m *marmot) StartVm(c *gin.Context) {
	slog.Info("start vm", "etcd", m.EtcdUrl)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup config", "err", err)
		return
	}
	err = startVM(m.EtcdUrl, spec)
	if err != nil {
		slog.Error("start vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// VMの開始
func startVM(dbUrl string, spec cf.VMSpec) error {
	// 仮想マシンの開始
	url := "qemu:///system"
	err := virt.StartVM(url, spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}

	d, err := db.NewDatabase(dbUrl)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	vm, err := d.GetVmByKey(spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}

	if vm.Status == db.STOPPED {
		// ハイパーバイザーのリソースの減算と保存
		hv, err := d.GetHvByKey(vm.HvNode)
		if err != nil {
			slog.Error("", "err", err)
		}
		hv.FreeCpu = hv.FreeCpu - vm.Cpu
		hv.FreeMemory = hv.FreeMemory - vm.Memory

		err = d.PutDataEtcd(hv.Key, hv)
		if err != nil {
			slog.Error("", "err", err)
		}
		// データベースの更新
		err = d.UpdateVmState(spec.Key, db.RUNNING)
		if err != nil {
			slog.Error("", "err", err)
		}
	}
	return nil
}
