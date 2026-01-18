package marmotd

import (
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

// 仮想マシンの生成、qcow2に対応すること、仮想マシンを識別するIDは、ホスト名ではなくUUIDであることに注意
// volume の生成は、volumes.goに任せること！
func (m *Marmot) CreateServer(spec api.Server) (string, error) {
	slog.Debug("=====CreateServer()=====", "spec", spec)

	var vol api.Volume
	slog.Debug("OS指定がなければ、OSバリアントのデフォルトを設定")
	if spec.OsVariant == nil {
		vol.OsVariant = util.StringPtr("ubuntu22.04")
		spec.OsVariant = util.StringPtr("ubuntu22.04")
	}

	slog.Debug("仮想マシンの使用を付与してDBへ登録、一意のIDを取得")
	server, err := m.Db.CreateServer(spec)
	if err != nil {
		slog.Error("CreateServer()", "err", err)
		return "", err
	}

	slog.Debug("ブートボリュームの生成と設定")
	vol.Name = util.StringPtr("boot-" + server.Id)
	vol.Kind = util.StringPtr("os")
	vol.Path = util.StringPtr("")
	vol.Size = util.IntPtrInt(0)

	// ステータスを起動中に更新
	var status api.Server
	status.Status = util.IntPtrInt(db.SERVER_PROVISIONING)
	err = m.Db.UpdateServer(server.Id, status)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}

	slog.Debug("ボリュームタイプの指定がなければ、デフォルトqcow2を設定")
	if spec.BootVolumeType == nil {
		vol.Type = util.StringPtr("qcow2")
	} else {
		vol.Type = util.StringPtr(*spec.BootVolumeType)
	}
	volSpec, err := m.CreateNewVolume(vol)
	if err != nil {
		return "", err
	}

	// ブートボリュームのIDをサーバーに設定
	server.BootVolumeId = util.StringPtr(volSpec.Id)
	err = m.Db.UpdateServer(server.Id, server)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}

	slog.Debug("データボリュームの生成と設定")
	//////////////////////////////////
	if spec.Storage != nil {
		for i, disk := range *spec.Storage {
			slog.Debug("spec.Storage", "disk index", i)
			slog.Debug("spec.Storage", "disk size", disk.Size)
			slog.Debug("spec.Storage", "disk volume group", disk.VolumeGroup)
			slog.Debug("spec.Storage", "disk logical volume", disk.LogicalVolume)
			slog.Debug("spec.Storage", "disk type", disk.Type)
			slog.Debug("spec.Storage", "disk path", disk.Path)
			if disk.Size == nil {
				continue
			}
		}
	}

	slog.Debug("ネットワークインタフェースの設定")
	/////////////////////////////////

	slog.Debug("ハイパーバイザーのリソース確保")
	var vx virt.VmSpec
	vx.UUID = *server.Uuid
	if server.Name != nil {
		vx.Name = *server.Name // VMを一意に識別する
	} else {
		vx.Name = "vm-" + server.Id
	}

	// CPUとメモリの設定
	slog.Debug("割り当てるCPU数とメモリ量を設定")
	if server.Cpu != nil {
		vx.CountVCPU = uint(*server.Cpu)
	} else {
		vx.CountVCPU = 2 // デフォルト2
	}

	if server.Memory != nil {
		mem := uint(*server.Memory) * 1024 //MiB
		vx.RAM = mem
	} else {
		mem := uint(2048 * 1024) // MiB デフォルト2048MB
		vx.RAM = mem
	}
	vx.Machine = "pc-q35-4.2"

	// ブートディスクの設定
	slog.Debug("New Boot", "Volume ID:", volSpec.Id)
	slog.Debug("CreateServer()", "volSpec Type", *volSpec.Type)
	switch {
	case volSpec.Type == nil:
		slog.Error("CreateServer()", "unsupported volume type", "nil")
		return "", fmt.Errorf("unsupported volume type: nil")
	case *volSpec.Type == "qcow2":
		slog.Debug("CreateServer()", "volSpec key", *volSpec.Key)
		slog.Debug("CreateServer()", "volSpec Path", *volSpec.Path)
		vx.DiskSpecs = []virt.DiskSpec{
			{"vda", *volSpec.Path, 3, *volSpec.Type},
		}
	case *volSpec.Type == "lvm":
		slog.Debug("CreateServer()", "volSpec lv", *volSpec.VolumeGroup)
		slog.Debug("CreateServer()", "volSpec lv", *volSpec.LogicalVolume)
		lvPath := fmt.Sprintf("/dev/%s/%s", *volSpec.VolumeGroup, *volSpec.LogicalVolume)
		vx.DiskSpecs = []virt.DiskSpec{
			{"vda", lvPath, 3, "raw"},
		}
	default:
		slog.Error("CreateServer()", "unsupported volume type", *volSpec.Type)
		return "", fmt.Errorf("unsupported volume type: %s", *volSpec.Type)
	}

	// 作成途中
	//vx.DiskSpecs = []virt.DiskSpec{
	//	{"vda", "/dev/vg1/oslv", 3, "raw"},
	//	{"vdb", "/dev/vg1/lvdata", 10, "raw"},
	//	{"vdc", "/var/lib/libvirt/images/data-vol-1.qcow2", 11, "qcow2"},
	//	}

	channelFile := "org.qemu.guest_agent.0"
	channelPath, err := util.CreateChannelDir(vx.UUID)

	/*
		ネットワークの指定がなければ、デフォルトネットワークを使用する。
		ネットワークの指定があれば、そのネットワークへ接続する。
		ネットワークとIPアドレスの指定があれば、そのIPアドレスを使用する。
		ネットワークの指定があっても、IPアドレスの指定がなければ、DHCPで取得する。
	*/
	if spec.Network == nil {
		slog.Debug("ネットワーク指定なし、デフォルトネットワークを使用")
		mac, err := util.GenerateRandomMAC()
		if err != nil {
			slog.Error("GenerateRandomMAC()", "err", err)
			return "", err
		}
		vx.Nets = []virt.NetSpec{
			{
				MAC:     mac.String(),
				Network: "default",
				PortID:  uuid.New().String(),
				Bridge:  "virbr0",
				Target:  "vnet2",
				Alias:   "net0",
				Bus:     1,
			},
		}
	} else {
		slog.Debug("ネットワーク指定あり、指定されたネットワークを使用")
		for i, nic := range *spec.Network {
			slog.Debug("ネットワーク", "index", i, "network id", nic.Id)
			mac, err := util.GenerateRandomMAC()
			if err != nil {
				slog.Error("GenerateRandomMAC()", "err", err)
				return "", err
			}
			ns := virt.NetSpec{
				MAC:     mac.String(),
				Network: nic.Id,
				PortID:  uuid.New().String(),
				Bridge:  "virbr0",
				Target:  fmt.Sprintf("vnet%d", i),
				Alias:   fmt.Sprintf("net%d", i),
				Bus:     uint(i + 1),
			}
			vx.Nets = append(vx.Nets, ns)
		}
	}
	vx.ChannelSpecs = []virt.ChannelSpec{
		{"unix", channelPath + "/" + channelFile, channelFile, "channel0", 1},
		{"spicevmc", "", "com.redhat.spice.0", "channel1", 2},
	}
	vx.Clocks = []virt.ClockSpec{
		{"rtc", "catchup", ""},
		{"pit", "delay", ""},
		{"hpet", "", "no"},
	}

	dom := virt.CreateDomainXML(vx)
	xml, err := dom.Marshal()
	fmt.Println("Generated", "libvirt XML:\n", string(xml))

	l, err := virt.NewLibVirtEp("qemu:///system")
	if err != nil {
		slog.Error("NewLibVirtEp()", "err", err)
		return "", err
	}
	defer l.Close()

	slog.Debug("仮想マシンの定義と起動")

	err = l.DefineAndStartVM(*dom)
	if err != nil {
		slog.Error("DefineAndStartVM()", "err", err)
		return "", err
	}

	// ステータスを利用可能に更新
	status.Status = util.IntPtrInt(db.SERVER_AVAILABLE)
	err = m.Db.UpdateServer(server.Id, status)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}

	return server.Id, nil
}

// 仮想マシンの削除、qcow2に対応すること、仮想マシンを識別するIDは、ホスト名ではなくUUIDであることに注意
// volume の生成は、volumes.goに任せること！
func (m *Marmot) DeleteServerById(id string) error {
	slog.Debug("===DeleteServerById is called===", "id", id)
	sv, err := m.GetServerById(id)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return err
	}

	slog.Debug("DeleteServerById()", "boot volume id", *sv.BootVolumeId)

	// 仮想マシンの削除
	l, err := virt.NewLibVirtEp("qemu:///system")
	if err != nil {
		slog.Error("NewLibVirtEp()", "err", err)
		return err
	}
	defer l.Close()
	if err = l.DeleteDomain(*sv.Name); err != nil {
		slog.Error("DeleteDomain()", "err", err)
		return err
	}

	// ブートボリュームの削除
	if err := m.RemoveVolume(*sv.BootVolumeId); err != nil {
		slog.Error("RemoveVolume()", "err", err)
		return err
	}

	// ボリュームを消す
	slog.Debug("アタッチされているボリュームの削除")
	if sv.Storage != nil {
		for i, stg := range *sv.Storage {
			slog.Debug("DeleteServerById()", "deleting volume", i, "volume id", stg.Id)
			if err := m.RemoveVolume(stg.Id); err != nil {
				slog.Error("DeleteVolumeById()", "err", err)
				return err
			}
		}
	}

	slog.Debug("DeleteServerById()", "sv", sv.Id, "name", *sv.Name)
	if err := m.Db.DeleteServerById(id); err != nil {
		slog.Error("DeleteServerById()", "err", err)
		return err
	}

	return nil
}

// サーバーのリストを取得、フィルターは、パラメータで指定するようにする
func (m *Marmot) GetServers() (api.Servers, error) {
	slog.Debug("===GetServers is called===", "id", "")
	svc, err := m.Db.GetServers()
	if err != nil {
		slog.Error("GetServers()", "err", err)
		return nil, err
	}
	slog.Debug("GetServers()", "Number of servers", len(svc))
	return svc, nil
}

// サーバーの詳細を取得
func (m *Marmot) GetServerById(id string) (api.Server, error) {
	slog.Debug("===GetServerById is called===", "id", id)
	server, err := m.Db.GetServerById(id)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return api.Server{}, err
	}
	slog.Debug("GetServerById()", "server boot volume id", server.BootVolumeId)
	slog.Debug("GetServerById()", "server", server)

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
