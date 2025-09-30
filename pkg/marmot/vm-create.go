package marmot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/takara9/marmot/api"
	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

// VMの作成
func (m *Marmot) CreateVm(c *gin.Context) {
	slog.Info("create vm", "etcd", m.EtcdUrl)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("create vm in action", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	err = m.createVM(spec)
	if err != nil {
		slog.Error("creating vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// VMを生成する
func (m *Marmot) createVM(spec cf.VMSpec) error {
	_, err := os.Executable()
	if err != nil {
		return err
	}

	var dom virt.Domain
	err = virt.ReadXml("temp.xml", &dom)
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

	osLogicalVol, err := util.CreateOsLv(m.EtcdUrl, spec.OsTempVg, spec.OsTempLv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	dom.Devices.Disk[0].Source.Dev = fmt.Sprintf("/dev/%s/%s", spec.OsTempVg, osLogicalVol)
	err = m.Db.UpdateOsLv(spec.Key, spec.OsTempVg, osLogicalVol)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	err = util.ConfigRootVol(spec, spec.OsTempVg, osLogicalVol)
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
		dlv, err := util.CreateDataLv(m.EtcdUrl, uint64(disk.Size), vg)
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
		err = m.Db.UpdateDataLv(spec.Key, i, disk.VolGrp, dlv)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}
		// エラー発生時にロールバックが必要（未実装）
	}

	// ストレージの更新
	util.CheckHvVG2(m.EtcdUrl, m.NodeName, spec.OsTempVg)
	if len(spec.PrivateIP) > 0 {
		util.CreateNic("pri", &dom.Devices.Interface)
	}

	if len(spec.PublicIP) > 0 {
		util.CreateNic("pub", &dom.Devices.Interface)
	}

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
	return nil
}

// 最終的に削除する方向
// リモートホストにリクエストを送信する
func createRemoteVM(hvNode string, port int, spec cf.VMSpec) error {
	byteJSON, _ := json.MarshalIndent(spec, "", "    ")
	// JSON形式でポストする
	reqURL := fmt.Sprintf("http://%s:%d/api/v1/%s", hvNode, port, "createVm")
	request, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		slog.Error("newRequest", "err", err)
		return err
	}

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		slog.Error("client.Do", "err", err)
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

// VMを生成する
func (m *Marmot) CreateVM2(spec api.VmSpec) error {
	var dom virt.Domain

	// ファイル名までのフルパスが exe に格納される
	_, err := os.Executable()
	if err != nil {
		return err
	}

	err = virt.ReadXml("temp.xml", &dom)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	dom.Name = *spec.Key // VMを一意に識別するキーでありhostnameではない
	dom.Uuid = *spec.Uuid
	dom.Vcpu.Value = int(*spec.Cpu)
	var mem = int(*spec.Memory) * 1024 //KiB
	dom.Memory.Value = mem
	dom.CurrentMemory.Value = mem

	osLogicalVol, err := util.CreateOsLv(m.EtcdUrl, *spec.Ostempvg, *spec.Ostemplv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	dom.Devices.Disk[0].Source.Dev = fmt.Sprintf("/dev/%s/%s", *spec.Ostempvg, osLogicalVol)
	err = m.Db.UpdateOsLv(*spec.Key, *spec.Ostempvg, osLogicalVol)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	err = util.ConfigRootVol2(spec, *spec.Ostempvg, osLogicalVol)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	if spec.Storage == nil {
		return nil
	}

	// DATAボリュームを作成 (最大９個)
	dev := []string{"vdb", "vdc", "vde", "vdf", "vdg", "vdh", "vdj", "vdk", "vdl"}
	bus := []string{"0x0a", "0x0b", "0x0c", "0x0d", "0x0e", "0x0f", "0x10", "0x11", "0x12"}
	for i, disk := range *spec.Storage {
		var dk virt.Disk
		// ボリュームグループが指定されていない時はvg1を指定
		var vg string = "vg1"
		if len(*disk.Vg) > 0 {
			vg = *disk.Vg
		}
		dlv, err := util.CreateDataLv(m.EtcdUrl, uint64(*disk.Size), vg)
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
		err = m.Db.UpdateDataLv(*spec.Key, i, *disk.Vg, dlv)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}
		// エラー発生時にロールバックが必要（未実装）
	}

	// ストレージの更新
	util.CheckHvVG2(m.EtcdUrl, m.NodeName, *spec.Ostempvg)
	if len(*spec.PrivateIp) > 0 {
		util.CreateNic("pri", &dom.Devices.Interface)
	}

	if len(*spec.PublicIp) > 0 {
		util.CreateNic("pub", &dom.Devices.Interface)
	}

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
	return nil
}
