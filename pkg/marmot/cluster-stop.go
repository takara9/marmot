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

// クラスタの停止
func (m *marmot) StopCluster(c *gin.Context) {
	slog.Info("stop cluster", "etcd", m.EtcdUrl)

	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("setup Json", "err", err)
		c.JSON(400, gin.H{"msg": "Can't read JSON"})
		return
	}
	if err := m.stopCluster(cnf); err != nil {
		slog.Error("stop cluster", "err", err)
		return
	}
}

// クラスタ停止
func (m *marmot) stopCluster(cnf cf.MarmotConfig) error {
	var NotFound bool = true
	for _, spec := range cnf.VMSpec {
		vmKey, _ := m.Db.FindByHostAndClusteName(spec.Name, cnf.ClusterName)
		if len(vmKey) > 0 {
			NotFound = false
			spec.Key = vmKey
			vm, err := m.Db.GetVmByKey(vmKey)
			if err != nil {
				slog.Error("", "err", err)
				continue
			}
			err = remoteStopVM(vm.HvNode, spec)
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

func remoteStopVM(hvNode string, spec cf.VMSpec) error {
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

// 仮想マシンの停止
func (m *marmot) StopVm(c *gin.Context) {
	slog.Info("stop vm", "etcd", m.EtcdUrl)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup config", "err", err)
		return
	}
	err = m.stopVM(spec)
	if err != nil {
		slog.Error("stop vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// VMの停止
func (m *marmot) stopVM(spec cf.VMSpec) error {
	vm, err := m.Db.GetVmByKey(spec.Key)
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
		hv, err := m.Db.GetHvByKey(vm.HvNode)
		if err != nil {
			slog.Error("", "err", err)
		}
		hv.FreeCpu = hv.FreeCpu + vm.Cpu
		hv.FreeMemory = hv.FreeMemory + vm.Memory
		err = m.Db.PutDataEtcd(hv.Key, hv)
		if err != nil {
			slog.Error("", "err", err)
		}
		// データベースの更新
		err = m.Db.UpdateVmState(spec.Key, db.STOPPED) ////////
		if err != nil {
			slog.Error("", "err", err)
		}
	}
	return nil
}
