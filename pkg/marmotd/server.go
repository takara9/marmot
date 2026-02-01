package marmotd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

// 仮想マシンの生成、qcow2に対応すること、仮想マシンを識別するIDは、ホスト名ではなくUUIDであることに注意
// volume の生成は、volumes.goに任せること！
func (m *Marmot) CreateServer(requestServerSpec api.Server) (string, error) {
	slog.Debug("=====CreateServer()=====", "config spec", requestServerSpec)

	var bootVol api.Volume
	slog.Debug("OS指定がなければ、OSバリアントのデフォルトを設定")
	if requestServerSpec.OsVariant == nil {
		bootVol.OsVariant = util.StringPtr("ubuntu22.04")
		requestServerSpec.OsVariant = util.StringPtr("ubuntu22.04")
	}

	slog.Debug("仮想マシンの使用を付与してDBへ登録、一意のIDを取得")
	serverConfig, err := m.Db.CreateServer(requestServerSpec)
	if err != nil {
		slog.Error("CreateServer()", "err", err)
		return "", err
	}

	slog.Debug("ブートボリュームの生成と設定")
	bootVol.Name = util.StringPtr("boot-" + serverConfig.Id)
	bootVol.Kind = util.StringPtr("os")
	bootVol.Path = util.StringPtr("")
	bootVol.Size = util.IntPtrInt(0)

	// ステータスを起動中に更新
	serverConfig.Status = util.IntPtrInt(db.SERVER_PROVISIONING)
	err = m.Db.UpdateServer(serverConfig.Id, serverConfig)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}

	slog.Debug("** OSの種類が指定されていなければ、デフォルトを設定 ** ", "os_variant", requestServerSpec.OsVariant)
	if requestServerSpec.OsVariant == nil {
		serverConfig.OsVariant = util.StringPtr("ubuntu22.04")
	}

	slog.Debug("ボリュームタイプの指定がなければ、デフォルトqcow2を設定", "boot volume type", requestServerSpec.BootVolume)
	if requestServerSpec.BootVolume == nil {
		bootVol.Type = util.StringPtr("qcow2")
	} else {
		bootVol.Type = requestServerSpec.BootVolume.Type
		bootVol.OsVariant = requestServerSpec.OsVariant
	}

	slog.Debug("ブートディスクにOSの指定がなければ、デフォルトのOSを設定")
	if requestServerSpec.OsVariant == nil {
		bootVol.OsVariant = util.StringPtr("ubuntu22.04") // デフォルトをコンフィグに持たせるべき？
	} else {
		bootVol.OsVariant = requestServerSpec.OsVariant
	}

	slog.Debug("ブートディスクの作成")
	bootVolDefined, err := m.CreateNewVolume(bootVol)
	if err != nil {
		return "", err
	}

	slog.Debug("ブートボリュームのIDをサーバーの構成データに設定", "temp var volume id", bootVolDefined)
	serverConfig.BootVolume = bootVolDefined
	err = m.Db.UpdateServer(serverConfig.Id, serverConfig)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}
	slog.Debug("ブートボリュームのIDをサーバーの構成データに設定完了", "server boot volume", serverConfig.BootVolume)

	// ブートボリュームをマウントして、ホスト名、netplanを設定する
	slog.Debug("ブートボリュームをマウントして、ホスト名、netplanを設定する")
	if err := util.SetupLinux(serverConfig); err != nil {
		slog.Error("SetupLinux()", "err", err)
		return "", err
	}

	// データボリュームの作成
	slog.Debug("データボリュームの生成")
	if requestServerSpec.Storage != nil {
		for i, disk := range *requestServerSpec.Storage {
			if len(disk.Id) > 0 {
				slog.Debug("既存ボリュームを使用", "disk index", i, "volume id", disk.Id)
				diskVol, err := m.GetVolumeById(disk.Id)
				if err != nil {
					slog.Error("GetVolumeById()", "err", err)
					return "", err
				}

				// 永続フラグを立てる
				var peersistent bool = true
				diskVol.Persistent = &peersistent

				slog.Debug("既存ボリュームの情報取得成功", "disk index", i, "volume id", diskVol.Id, "path", diskVol.Path, "status", diskVol.Status)
				(*serverConfig.Storage)[i] = *diskVol
				slog.Debug("既存ボリュームの情報設定成功", "disk index", i, "volume id", diskVol.Id, "disk", disk)
				continue
			}

			if disk.Type != nil && *disk.Type == "qcow2" {
				slog.Debug("qcow2ボリュームを作成", "disk index", i)
				diskVol, err := m.CreateNewVolume(disk)
				if err != nil {
					slog.Error("CreateNewVolume()", "err", err)
					return "", err
				}
				(*serverConfig.Storage)[i] = *diskVol
				slog.Debug("データボリューム 作成成功", "disk index", i, "volume id", diskVol.Id)
			}
			if disk.Type != nil && *disk.Type == "lvm" {
				slog.Debug("lvmボリュームを作成", "disk index", i)
				diskVol, err := m.CreateNewVolume(disk)
				if err != nil {
					slog.Error("CreateNewVolume()", "err", err)
					return "", err
				}
				(*serverConfig.Storage)[i] = *diskVol
				slog.Debug("データボリューム 作成成功", "disk index", i, "volume id", diskVol.Id)
			}
		}
	}

	fmt.Println("=== データボリュームの情報確認2 ===", "server Id", serverConfig.Id)
	data3, err := json.MarshalIndent(serverConfig, "", "  ")
	if err != nil {
		slog.Error("json.MarshalIndent()", "err", err)
	} else {
		fmt.Println("サーバー情報(serverConfig): ", string(data3))
	}

	// データボリュームのIDをサーバーに設定
	err = m.Db.UpdateServer(serverConfig.Id, serverConfig)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}

	slog.Debug("ハイパーバイザーのリソース確保")
	var virtSpec virt.VmSpec
	virtSpec.UUID = *serverConfig.Uuid
	if serverConfig.Name != nil {
		virtSpec.Name = *serverConfig.Name + "-" + serverConfig.Id // VMを一意に識別する
	} else {
		virtSpec.Name = "vm-" + serverConfig.Id
	}
	// サーバーのVM名前をセットし、今後の操作のためにDBを更新する必要がある
	serverConfig.InstanceName = util.StringPtr(virtSpec.Name)

	// CPUとメモリの設定
	slog.Debug("割り当てるCPU数とメモリ量を設定")
	if serverConfig.Cpu != nil {
		virtSpec.CountVCPU = uint(*serverConfig.Cpu)
	} else {
		virtSpec.CountVCPU = 2 // デフォルト2
	}

	if serverConfig.Memory != nil {
		mem := uint(*serverConfig.Memory) * 1024 //MiB
		virtSpec.RAM = mem
	} else {
		mem := uint(2048 * 1024) // MiB デフォルト2048MB
		virtSpec.RAM = mem
	}
	virtSpec.Machine = "pc-q35-4.2"

	slog.Debug("ボリュームの設定が無いときはqcow2をデフォルトとする1")
	if bootVolDefined.Type == nil {
		bootVolDefined.Type = util.StringPtr("qcow2")
	}
	slog.Debug("ボリュームの設定が無いときはqcow2をデフォルトとする2", "boot volume ptr", bootVolDefined)

	switch {
	case *bootVolDefined.Type == "qcow2":
		virtSpec.DiskSpecs = []virt.DiskSpec{
			{"vda", *bootVolDefined.Path, 3, *bootVolDefined.Type},
		}
	case *bootVolDefined.Type == "lvm":
		// ＊＊＊　パスは createNewVolume で設定されるべき　＊＊＊
		lvPath := fmt.Sprintf("/dev/%s/%s", *bootVolDefined.VolumeGroup, *bootVolDefined.LogicalVolume)
		virtSpec.DiskSpecs = []virt.DiskSpec{
			{"vda", lvPath, 3, "raw"},
		}
	default:
		slog.Error("CreateServer()", "unsupported volume type", *bootVolDefined.Type)
		return "", fmt.Errorf("unsupported volume type: %s", *bootVolDefined.Type)
	}

	// データディスクの設定
	if serverConfig.Storage != nil {
		for i, disk := range *serverConfig.Storage {
			if disk.Kind == nil {
				disk.Kind = util.StringPtr("data")
			}
			if disk.Type == nil {
				disk.Type = util.StringPtr("qcow2")
			}
			switch {
			case *disk.Type == "qcow2":
				ds := virt.DiskSpec{
					Dev:  fmt.Sprintf("vd%c", 'b'+i),
					Src:  *disk.Path,
					Bus:  uint(11 + i),
					Type: "qcow2",
				}
				virtSpec.DiskSpecs = append(virtSpec.DiskSpecs, ds)
			case *disk.Type == "lvm":
				ds := virt.DiskSpec{
					Dev:  fmt.Sprintf("vd%c", 'b'+i),
					Src:  *disk.Path,
					Bus:  uint(11 + i),
					Type: "raw",
				}
				virtSpec.DiskSpecs = append(virtSpec.DiskSpecs, ds)
			}
		}
	}

	channelFile := "org.qemu.guest_agent.0"
	channelPath, err := util.CreateChannelDir(virtSpec.UUID)

	// ネットワークの設定
	if len(*requestServerSpec.Network) == 0 {
		slog.Debug("ネットワーク指定なし、デフォルトネットワークを使用")
		mac, err := util.GenerateRandomMAC()
		if err != nil {
			slog.Error("GenerateRandomMAC()", "err", err)
			return "", err
		}
		virtSpec.NetSpecs = []virt.NetSpec{
			{
				MAC:     mac.String(),
				Network: "default",
				PortID:  uuid.New().String(),
				Bridge:  "virbr0",
				Bus:     1,
			},
		}
		// サーバーのネットワーク情報を更新
		var net api.Network
		net.Id = virtSpec.NetSpecs[0].Network
		net.Mac = &virtSpec.NetSpecs[0].MAC
		serverConfig.Network = &[]api.Network{net}
	} else {
		slog.Debug("ネットワーク指定あり、指定されたネットワークを使用")
		for i, nic := range *requestServerSpec.Network {
			slog.Debug("ネットワーク", "index", i, "network id", nic.Id)
			mac, err := util.GenerateRandomMAC()
			if err != nil {
				slog.Error("GenerateRandomMAC()", "err", err)
				return "", err
			}
			busno := uint(i + 1)
			if busno >= 3 {
				busno += 4 // diskとバス番号が被らないようにする
			}
			ns := virt.NetSpec{
				MAC:     mac.String(),
				Network: nic.Id,
				PortID:  uuid.New().String(),
				Bridge:  "virbr0",
				Bus:     busno,
			}
			var ni api.Network
			virtSpec.NetSpecs = append(virtSpec.NetSpecs, ns)
			ni.Id = ns.Network
			ni.Mac = &ns.MAC
			// netplanで静的IPアドレスを設定する場合のために、IPアドレス情報もサーバーに保存しておく
			ni.Address = nic.Address
			ni.Netmask = nic.Netmask
			ni.Routes = nic.Routes
			ni.Nameservers = nic.Nameservers
			(*serverConfig.Network)[i] = ni
		}
	}
	// サーバーのネットワーク情報を更新
	err = m.Db.UpdateServer(serverConfig.Id, serverConfig)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}

	virtSpec.ChannelSpecs = []virt.ChannelSpec{
		{"unix", channelPath + "/" + channelFile, channelFile, "channel0", 1},
		{"spicevmc", "", "com.redhat.spice.0", "channel1", 2},
	}
	virtSpec.Clocks = []virt.ClockSpec{
		{"rtc", "catchup", ""},
		{"pit", "delay", ""},
		{"hpet", "", "no"},
	}

	dom := virt.CreateDomainXML(virtSpec)
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
	createTime := time.Now()
	serverConfig.CTime = &createTime

	// ステータスを利用可能に更新
	serverConfig.Status = util.IntPtrInt(db.SERVER_AVAILABLE)
	err = m.Db.UpdateServer(serverConfig.Id, serverConfig)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}

	return serverConfig.Id, nil
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

	slog.Debug("DeleteServerById()", "boot volume id", sv.BootVolume.Id)

	// 仮想マシンの削除
	l, err := virt.NewLibVirtEp("qemu:///system")
	if err != nil {
		slog.Error("NewLibVirtEp()", "err", err)
		return err
	}
	defer l.Close()

	if sv.InstanceName != nil {
		slog.Debug("DeleteServerById()", "deleting domain", *sv.InstanceName)
		if err = l.DeleteDomain(*sv.InstanceName); err != nil {
			// ドメインが存在しない場合はスキップしたいが、区別が難しいので意図的にスキップする
			//if *sv.Status != db.SERVER_PROVISIONING {
			//	slog.Error("DeleteDomain()", "err", err)
			//	return err
			//}
			slog.Debug("DeleteServerById()", "server is in PROVISIONING state, skipping domain deletion", *sv.Name)
			// return nil 戻さず、削除処理を続行する
		}
	} else {
		slog.Debug("DeleteServerById() no instance name set, skipping domain deletion", "server name", *sv.Name)
	}

	// ブートボリュームの削除
	if err := m.RemoveVolume(sv.BootVolume.Id); err != nil {
		if err == db.ErrNotFound {
			slog.Debug("RemoveVolume() boot volume already deleted", "volume id", sv.BootVolume.Id)
		} else {
			slog.Error("RemoveVolume()", "err", err)
			return err
		}
	}

	// データボリュームを消す
	if sv.Storage != nil {
		slog.Debug("アタッチされているボリュームの削除", "ボリューム数", len(*sv.Storage))
		for i, vol := range *sv.Storage {
			slog.Debug("DeleteServerById()", "index", i, "deleting volume id", vol.Id)
			if vol.Persistent != nil && *vol.Persistent {
				slog.Debug("DeleteServerById()", "skipping persistent volume", vol.Id)
				continue
			}
			if err := m.RemoveVolume(vol.Id); err != nil {
				if err == db.ErrNotFound {
					slog.Debug("RemoveVolume() data volume already deleted", "volume id", vol.Id)
					continue
				} else {
					slog.Error("RemoveVolume()", "err", err)
					return err
				}
			}
		}
	} else {
		slog.Debug("DeleteServerById()", "no attached volumes to delete", sv.Id)
	}

	slog.Debug("DeleteServerById()", "sv", sv.Id, "name", *sv.Name)
	if err := m.Db.DeleteServerById(sv.Id); err != nil {
		slog.Error("DeleteServerById()", "err", err)
		return err
	}

	return nil
}

// サーバーのリストを取得、フィルターは、パラメータで指定するようにする
func (m *Marmot) GetServers() (api.Servers, error) {
	slog.Debug("===GetServers is called===", "none", "none")
	serverSpec, err := m.Db.GetServers()
	if err != nil {
		slog.Error("GetServers()", "err", err)
		return nil, err
	}
	slog.Debug("GetServers()", "Number of servers", len(serverSpec))
	return serverSpec, nil
}

// サーバーの詳細を取得
func (m *Marmot) GetServerById(id string) (api.Server, error) {
	slog.Debug("===GetServerById is called===", "id", id)
	serverSpec, err := m.Db.GetServerById(id)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return api.Server{}, err
	}
	// ここで BootVolumeのIDがセットできていない理由を調べる!
	slog.Debug("GetServerById()", "server boot volume id", serverSpec.BootVolume.Id)
	slog.Debug("GetServerById()", "server", serverSpec)

	return serverSpec, nil
}

// サーバーの更新
func (m *Marmot) UpdateServerById(id string, serverSpec api.Server) error {
	slog.Debug("===", "UpdateServerById is called", "===")
	err := m.Db.UpdateServer(id, serverSpec)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return err
	}
	slog.Debug("UpdateServerById()", "svc", nil)
	return nil
}
