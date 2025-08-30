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
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

// コールバック VMクラスタの削除
func (m *marmot) DestroyCluster(c *gin.Context) {
	var cnf cf.MarmotConfig
	if err := c.BindJSON(&cnf); err != nil {
		slog.Error("prepare to delete cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	if err := m.destroyCluster(cnf); err != nil {
		slog.Error("delete cluster", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// クラスタ削除
func (m *marmot) destroyCluster(cnf cf.MarmotConfig) error {
	var NotFound bool = true
	for _, spec := range cnf.VMSpec {
		// クラスタ名とホスト名の重複チェック
		vmKey, _ := m.Db.FindByHostAndClusteName(spec.Name, cnf.ClusterName)
		fmt.Println("DEBUG Print in DestroyCluster vmKey, specName", vmKey, spec.Name)
		if len(vmKey) > 0 {
			NotFound = false
			spec.Key = vmKey
			vm, err := m.Db.GetVmByKey(vmKey)
			if err != nil {
				slog.Error("", "err", err)
				continue
			}
			err = remoteDestroyVM(vm.HvNode, spec)
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
func remoteDestroyVM(hvNode string, spec cf.VMSpec) error {
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
func (m *marmot) DestroyVm(c *gin.Context) {
	slog.Info("destroy vm", "etcd", m.EtcdUrl)

	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup spec", "err", err)
		return
	}
	err = m.destroyVM(spec)
	if err != nil {
		slog.Error("delete vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// VMの削除
func (m *marmot) destroyVM(spec cf.VMSpec) error {
	vm, err := m.Db.GetVmByKey(spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}

	// ハイパーバイザーのリソース削減保存のため値を取得
	hv, err := m.Db.GetHvByKey(vm.HvNode)
	if err != nil {
		slog.Error("", "err", err)
	}

	// ステータスを調べて停止中であれば、足し算しない。
	if vm.Status != db.STOPPED || vm.Status == db.ERROR {
		hv.FreeCpu = hv.FreeCpu + vm.Cpu
		hv.FreeMemory = hv.FreeMemory + vm.Memory
		err = m.Db.PutDataEtcd(hv.Key, hv)
		if err != nil {
			slog.Error("", "err", err)
		}
	}

	// データベースから削除
	err = m.Db.DelByKey(spec.Key)
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
	util.CheckHvVG2(m.EtcdUrl, m.NodeName, vm.OsVg)

	// データLVを削除
	for _, dd := range vm.Storage {
		err = lvm.RemoveLV(dd.Vg, dd.Lv)
		if err != nil {
			slog.Error("", "err", err)
		}
		// ストレージの更新
		util.CheckHvVG2(m.EtcdUrl, m.NodeName, dd.Vg)
	}
	return nil
}
