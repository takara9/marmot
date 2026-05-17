package marmotd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/networkfabric"
	"github.com/takara9/marmot/pkg/qcow"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

func findHostStatusByNodeName(statuses []api.HostStatus, nodeName string) *api.HostStatus {
	node := strings.TrimSpace(nodeName)
	if node == "" {
		return nil
	}
	for i := range statuses {
		if statuses[i].NodeName == nil {
			continue
		}
		if strings.TrimSpace(*statuses[i].NodeName) == node {
			return &statuses[i]
		}
	}
	return nil
}

func normalizeISCSITargetName(targetIQN string) string {
	t := strings.TrimSpace(targetIQN)
	if t == "" {
		return ""
	}
	if strings.Contains(t, "/") {
		return t
	}
	return t + "/0"
}

func resolveISCSIServerNode(statuses []api.HostStatus) string {
	active := filterActiveHosts(statuses)
	if len(active) == 0 {
		return ""
	}

	// 明示設定がある場合はその中から決定的に1つ選ぶ
	type candidate struct {
		nodeName string
		hostID   uint32
	}
	var explicits []candidate
	for _, s := range active {
		if s.IscsiServer == nil || !*s.IscsiServer || s.NodeName == nil || s.HostId == nil {
			continue
		}
		node := strings.TrimSpace(*s.NodeName)
		if node == "" {
			continue
		}
		hid, ok := parseHostIDHex(*s.HostId)
		if !ok {
			continue
		}
		explicits = append(explicits, candidate{nodeName: node, hostID: hid})
	}
	if len(explicits) > 0 {
		sort.Slice(explicits, func(i, j int) bool {
			if explicits[i].hostID != explicits[j].hostID {
				return explicits[i].hostID < explicits[j].hostID
			}
			return explicits[i].nodeName < explicits[j].nodeName
		})
		return explicits[0].nodeName
	}

	for _, s := range active {
		if s.NodeName == nil {
			continue
		}
		node := strings.TrimSpace(*s.NodeName)
		if node == "" {
			continue
		}
		if IsSchedulerLeader(node, statuses) {
			return node
		}
	}

	return ""
}

func (m *Marmot) resolveISCSIDiskAttachment(nodeName string, disk api.Volume) (targetName, host, port, initiator string, err error) {
	if m == nil || m.Db == nil {
		return "", "", "", "", errors.New("marmot db is nil")
	}
	if disk.Spec.IscsiTargetIqn == nil {
		return "", "", "", "", errors.New("iscsi target iqn is missing")
	}

	statuses, err := m.Db.GetAllHostStatus()
	if err != nil {
		return "", "", "", "", err
	}

	iscsiServerNode := resolveISCSIServerNode(statuses)
	if iscsiServerNode == "" {
		return "", "", "", "", errors.New("failed to resolve iscsi server node")
	}
	iscsiServerStatus := findHostStatusByNodeName(statuses, iscsiServerNode)
	if iscsiServerStatus == nil || iscsiServerStatus.IpAddress == nil || strings.TrimSpace(*iscsiServerStatus.IpAddress) == "" {
		return "", "", "", "", fmt.Errorf("iscsi server hoststatus ip is missing: %s", iscsiServerNode)
	}

	initiator = strings.TrimSpace(getISCSIInitiatorID())
	if initiator == "" {
		vmHostStatus := findHostStatusByNodeName(statuses, nodeName)
		if vmHostStatus != nil && vmHostStatus.InitiatorId != nil {
			initiator = strings.TrimSpace(*vmHostStatus.InitiatorId)
		}
	}
	if initiator == "" {
		return "", "", "", "", fmt.Errorf("iscsi initiator id is missing on vm host: %s", strings.TrimSpace(nodeName))
	}

	return normalizeISCSITargetName(*disk.Spec.IscsiTargetIqn), strings.TrimSpace(*iscsiServerStatus.IpAddress), "3260", initiator, nil
}

// サーバーの生成 コントローラーから呼び出される
func (m *Marmot) CreateServerManage(id string) (string, error) {
	slog.Debug("=====CreateServer2()=====", "", "")

	var bootVol api.Volume
	var bootVolSpec api.VolSpec
	var bootVolMeta api.Metadata
	bootVol.Spec = bootVolSpec
	bootVol.Metadata = bootVolMeta
	var virtSpec virt.ServerSpec

	bootVol.ApiVersion = "v1"
	bootVol.Kind = "Volume"

	serverConfig, err := m.Db.GetServerById(id)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return "", err
	}

	assignedNodeName := strings.TrimSpace(m.NodeName)
	if serverConfig.Metadata.NodeName != nil {
		if node := strings.TrimSpace(*serverConfig.Metadata.NodeName); node != "" {
			assignedNodeName = node
		}
	}
	assignNodeNameIfUnset(&serverConfig.Metadata, assignedNodeName)

	slog.Debug("OS指定がなければ、OSバリアントのデフォルトを設定")
	if serverConfig.Spec.OsVariant == nil {
		bootVol.Spec.OsVariant = util.StringPtr("ubuntu22.04")
		serverConfig.Spec.OsVariant = util.StringPtr("ubuntu22.04")
	}

	slog.Debug("ブートボリュームの生成と設定")
	bootVol.Metadata.Name = "boot-" + api.ServerID(serverConfig)
	// サーバー割当ノードをブートボリュームのメタデータに付与する。
	assignNodeNameIfUnset(&bootVol.Metadata, assignedNodeName)
	bootVol.Spec.Kind = util.StringPtr("os")
	bootVol.Spec.Path = util.StringPtr("")
	bootVol.Spec.Size = util.IntPtrInt(0)

	slog.Debug("** OSの種類が指定されていなければ、デフォルトを設定 ** ", "os_variant", serverConfig.Spec.OsVariant)
	if serverConfig.Spec.OsVariant == nil {
		serverConfig.Spec.OsVariant = util.StringPtr("ubuntu22.04")
	}

	slog.Debug("ボリュームタイプの指定がなければ、デフォルトqcow2を設定", "boot volume type", serverConfig.Spec.BootVolume)
	if serverConfig.Spec.BootVolume == nil || serverConfig.Spec.BootVolume.Spec.Type == nil {
		bootVol.Spec.Type = util.StringPtr("qcow2")
	} else {
		bootVol.Spec.Type = serverConfig.Spec.BootVolume.Spec.Type
		bootVol.Spec.OsVariant = serverConfig.Spec.OsVariant
	}

	if bootVol.Spec.Type != nil && *bootVol.Spec.Type != "qcow2" &&
		serverConfig.Spec.BootVolume != nil &&
		serverConfig.Spec.BootVolume.Spec.Size != nil {
		return "", errors.New("boot_volume.size は boot_volume.type=qcow2 のときのみ指定できます")
	}

	if bootVol.Spec.Type != nil && *bootVol.Spec.Type == "qcow2" &&
		serverConfig.Spec.BootVolume != nil &&
		serverConfig.Spec.BootVolume.Spec.Size != nil && *serverConfig.Spec.BootVolume.Spec.Size > 0 {
		bootVol.Spec.Size = util.IntPtrInt(*serverConfig.Spec.BootVolume.Spec.Size)
	}

	slog.Debug("ブートディスクにOSの指定がなければ、デフォルトのOSを設定")
	if serverConfig.Spec.OsVariant == nil {
		bootVol.Spec.OsVariant = util.StringPtr("ubuntu22.04") // デフォルトをコンフィグに持たせるべき？
	} else {
		bootVol.Spec.OsVariant = serverConfig.Spec.OsVariant
	}

	slog.Debug("サーバーのネットワークインターフェースの設定")

	// ネットワークの設定
	if serverConfig.Spec.NetworkInterface == nil {
		// ネットワーク指定なし、デフォルトネットワークを使用
		slog.Debug("ネットワーク指定なし、デフォルトネットワークを使用")
		mac, err := util.GenerateRandomMAC()
		if err != nil {
			slog.Error("GenerateRandomMAC()", "err", err)
			return "", err
		}
		// サーバーのネットワーク情報を更新
		var net api.NetworkInterface

		// ネットワーク名から、ネットワークのIDを取得して、net.Networkidにセットする必要がある
		xnet, err := m.Db.GetVirtualNetworkByName("default")
		if err != nil {
			slog.Error("GetNetworkIdByName()", "err", err)
			return "", err
		}

		defaultNS := virt.NetSpec{
			MAC:     mac.String(),
			Network: xnet.Metadata.Name,
			PortID:  uuid.New().String(),
			Bus:     1,
		}
		// default ネットワークが libvirt 側に見えない環境向けのフォールバック。
		if strings.EqualFold(xnet.Metadata.Name, "default") && xnet.Spec.BridgeName != nil && strings.TrimSpace(*xnet.Spec.BridgeName) != "" {
			defaultNS.Bridge = strings.TrimSpace(*xnet.Spec.BridgeName)
			defaultNS.Network = ""
		}
		virtSpec.NetSpecs = []virt.NetSpec{defaultNS}

		net.Networkid = api.VirtualNetworkID(xnet)
		net.Networkname = xnet.Metadata.Name
		net.Mac = &virtSpec.NetSpecs[0].MAC
		serverConfig.Spec.NetworkInterface = &[]api.NetworkInterface{net}
	} else {
		slog.Debug("ネットワーク指定あり、指定されたネットワークを使用")
		for i, reqNic := range *serverConfig.Spec.NetworkInterface {
			slog.Debug("ネットワーク", "index", i, "network id", reqNic.Networkname)
			mac, err := util.GenerateRandomMAC()
			if err != nil {
				slog.Error("GenerateRandomMAC()", "err", err)
				return "", err
			}
			vnet, err := m.Db.GetVirtualNetworkByName(reqNic.Networkname)
			if err != nil {
				slog.Error("GetVirtualNetworkByName()", "err", err, "network name", reqNic.Networkname)
				if err.Error() == "not found" {
					err = fmt.Errorf("network '%s' is not found", reqNic.Networkname)
				}
				return "", err
			}

			var ipaddr string
			var bitmask int
			var ipnet *api.IPNetwork

			// DEBUG
			jsonBytes0, err := json.MarshalIndent(reqNic, "", "  ")
			if err != nil {
				slog.Error("failed to marshal network interface", "err", err)
			} else {
				fmt.Println("=== ネットワークインターフェースの情報 ===", "ni", string(jsonBytes0))
			}

			reqNic.Networkid = api.VirtualNetworkID(vnet) // ネットワークIDをセット
			if reqNic.Address != nil {
				// リクエストにIPアドレスが指定されている場合は、そのIPアドレスを使用する
				ipaddr = *reqNic.Address

				if reqNic.Netmasklen != nil {
					bitmask = *reqNic.Netmasklen
				} else if reqNic.Netmask != nil {
					// netmask が数値文字列（CIDRプレフィックス長）の場合、Netmasklen として使用する
					// 有効範囲: IPv4は0-32、IPv6は0-128
					if maskLen, err := strconv.Atoi(*reqNic.Netmask); err == nil && maskLen >= 0 && maskLen <= 128 {
						bitmask = maskLen
					} else {
						slog.Debug("Invalid or missing netmask format, using default 24", "netmask", *reqNic.Netmask)
						bitmask = 24 // デフォルトのビットマスク長
					}
				} else {
					slog.Debug("Netmask length is not specified, using default 24")
					bitmask = 24 // デフォルトのビットマスク長
				}
				//slog.Debug("IP address is specified in the request, skipping IP allocation", "ip address", ipaddr, "netmask length", *reqNic.Netmasklen)

				// IPネットワークが存在していれば、IPネットワークを作成する必要はない。
				// IPネットワークと IPアドレスを設定
				ipNetAddr := &api.IPNetwork{
					AddressMaskLen: util.StringPtr(fmt.Sprintf("%s/%d", ipaddr, bitmask)),
				}
				ipNetId, err := m.Db.CreateIpNetwork(api.VirtualNetworkID(vnet), ipNetAddr)
				if err != nil {
					if err.Error() == db.ErrAlreadyExists || err.Error() == db.ErrOverlapsExistingNetwork {
						//NOP
					} else {
						slog.Error("CreateIpNetwork()", "err", err)
						return "", err
					}
				}

				slog.Debug("IPネットワークの作成成功", "network id", api.VirtualNetworkID(vnet), "ip network id", ipNetId, "ip address with mask", ipaddr)
				// ネットワークインターフェースのIPネットワークIDを設定
				reqNic.IpNetworkId = util.StringPtr(ipNetId)
				slog.Debug("ネットワークインターフェースのIPネットワークIDを設定成功", "network id", api.VirtualNetworkID(vnet), "ip network id", ipNetId)
				// IPアドレスの使用済設定

				// 一致するものが無かったら、そのIPアドレスを割り当てる
				found, err := m.Db.CheckIPaddrInUse(api.VirtualNetworkID(vnet), ipNetId, ipaddr)
				if err != nil {
					slog.Error("AllocateIP()", "err", err, "vnetId", api.VirtualNetworkID(vnet), "ipnetId", ipNetId, "candidateIP", ipaddr)
					return "", err
				}
				if !found {
					slog.Debug("セットさられたIPアドレス", "IP	", ipaddr)
					m.Db.SetIPaddrInUse(api.VirtualNetworkID(vnet), ipNetId, ipaddr, serverConfig.Metadata.Name)
					//return ipaddr, nil
				}
				// 内部DNSへ登録
				slog.Debug("内部DNSへ登録", "hostname", serverConfig.Metadata.Name, "subdomain", reqNic.Networkname, "ip address", ipaddr)
				if err := m.Db.PutDnsEntry(serverConfig.Metadata.Name, reqNic.Networkname, ipaddr); err != nil {
					slog.Error("PutDnsEntry()", "err", err)
					return "", err
				}
			} else {
				// IPアドレスの指定が無いので、IPアドレスを割り当て
				slog.Debug("IPアドレスの割り当て", "network id", api.VirtualNetworkID(vnet), "network name", vnet.Metadata.Name)
				if vnet.Spec.IpNetworkId != nil {
					ipaddr, bitmask, err = m.Db.AllocateIP(api.VirtualNetworkID(vnet), *vnet.Spec.IpNetworkId, serverConfig.Metadata.Name)
					if err != nil {
						slog.Error("AllocateIP()", "err", err)
						return "", err
					}

					ipnet, err = m.Db.GetIpNetworkById(api.VirtualNetworkID(vnet), *vnet.Spec.IpNetworkId)
					if err != nil {
						slog.Error("GetIpNetworkById()", "err", err)
						return "", err
					}
					// 内部DNSへ登録
					slog.Debug("内部DNSへ登録", "hostname", serverConfig.Metadata.Name, "subdomain", reqNic.Networkname, "ip address", ipaddr)
					if err := m.Db.PutDnsEntry(serverConfig.Metadata.Name, reqNic.Networkname, ipaddr); err != nil {
						slog.Error("PutDnsEntry()", "err", err)
						return "", err
					}
				} else {
					slog.Debug("IPネットワークIDが指定されていないため、IPアドレスの割り当てができない", "network id", api.VirtualNetworkID(vnet), "network name", vnet.Metadata.Name)
				}
			}

			// ネットワークのバス番号は、ディスクのバス番号と被らないように、ディスクの数に応じて調整する
			busno := uint(i + 1)
			if busno >= 3 {
				busno += 4 // diskとバス番号が被らないようにする
			}
			ns := virt.NetSpec{
				MAC:     mac.String(),
				Network: vnet.Metadata.Name,
				PortID:  uuid.New().String(),
				Bus:     busno,
			}
			// 通常は libvirt network 名で接続する。
			// default のみ、環境差異により network が見えないケース向けに bridge へフォールバックする。
			if strings.EqualFold(vnet.Metadata.Name, "default") && vnet.Spec.BridgeName != nil && strings.TrimSpace(*vnet.Spec.BridgeName) != "" {
				ns.Bridge = strings.TrimSpace(*vnet.Spec.BridgeName)
				ns.Network = ""
			}

			// VLAN対応
			if reqNic.Portgroup != nil {
				ns.PortGroup = *reqNic.Portgroup
			}
			if reqNic.Vlans != nil && len(*reqNic.Vlans) > 0 {
				for _, v := range *reqNic.Vlans {
					ns.Vlans = append(ns.Vlans, v)
				}
			}
			virtSpec.NetSpecs = append(virtSpec.NetSpecs, ns)

			var ni api.NetworkInterface
			ni.Networkname = reqNic.Networkname
			ni.Networkid = api.VirtualNetworkID(vnet)

			// ここでIP Network Idがセットされた場合、データベースにも保存する必要がある
			if reqNic.IpNetworkId != nil {
				ni.IpNetworkId = reqNic.IpNetworkId
			}

			ni.Mac = &ns.MAC
			// IPアドレスとネットマスク長があれば、ネットワークインターフェースの情報にセットする
			if len(ipaddr) > 0 {
				ni.Address = util.StringPtr(ipaddr)
			}
			if bitmask > 0 {
				ni.Netmasklen = util.IntPtrInt(bitmask)
			}

			ni.Routes = reqNic.Routes
			ni.Nameservers = reqNic.Nameservers

			fmt.Println("=== ネットワークインターフェースの情報確認 ===", "network interface", "ipaddr", ipaddr, "bitmask", bitmask)

			// DEBUG
			jsonBytes, err := json.MarshalIndent(ni, "", "  ")
			if err != nil {
				slog.Error("failed to marshal network interface", "err", err)
			} else {
				fmt.Println("ネットワークインターフェースの情報", "ni", string(jsonBytes))
			}

			// DEBUG
			jsonBytes2, err := json.MarshalIndent(reqNic, "", "  ")
			if err != nil {
				slog.Error("failed to marshal requested network interface", "err", err)
			} else {
				fmt.Println("要求されたネットワークインターフェースの情報", "reqNic", string(jsonBytes2))
			}

			// netplanで静的IPアドレスを設定する場合のために、IPアドレス情報もサーバーに保存しておく
			if ipnet != nil && vnet.Spec.IpNetworkId != nil {
				// DEBUG
				jsonBytes3, err := json.MarshalIndent(*ipnet, "", "  ")
				if err != nil {
					slog.Error("failed to marshal IP network", "err", err)
				} else {
					fmt.Println("IPネットワークの情報", "ipnet", string(jsonBytes3))
				}

				if ipnet.Netmasklen != nil {
					ni.Netmasklen = util.IntPtrInt(*ipnet.Netmasklen)
				}
				if ipnet.Netmask != nil {
					ni.Netmask = util.StringPtr(*ipnet.Netmask)
				}
				// ルートとネームサーバーの情報も保存しておく
				if ipnet.Gateway != nil {
					ni.IpGateway = util.StringPtr(*ipnet.Gateway)
				}
				if vnet.Spec.IpNetworkId != nil {
					ni.IpNetworkId = util.StringPtr(*vnet.Spec.IpNetworkId)
				} else {
					ni.IpNetworkId = util.StringPtr(ni.Networkid)
				}
			}
			(*serverConfig.Spec.NetworkInterface)[i] = ni
		}
		// ループの終わり
	}
	// サーバーのネットワーク情報を更新
	err = m.Db.UpdateServer(api.ServerID(serverConfig), serverConfig)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}

	slog.Debug("ブートディスクの作成")
	bootVolDefined, err := m.CreateNewVolumeWithWait(bootVol)
	if err != nil {
		slog.Error("CreateNewVolumeWithWait()", "err", err)
		return "", err
	}

	slog.Debug("ブートボリュームのIDをサーバーの構成データに設定", "temp var volume id", api.VolumeID(bootVolDefined))
	serverConfig.Spec.BootVolume = &bootVolDefined
	err = m.Db.UpdateServer(api.ServerID(serverConfig), serverConfig)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}
	slog.Debug("ブートボリュームのIDをサーバーの構成データに設定完了", "server boot volume", serverConfig.Spec.BootVolume)

	// ブートボリュームをマウントして、ホスト名、netplanを設定する
	slog.Debug("ブートボリュームをマウントして、ホスト名、netplanを設定する")
	if err := util.SetupLinux(serverConfig); err != nil {
		slog.Error("SetupLinux()", "err", err)
		return "", err
	}

	// データボリュームの作成
	slog.Debug("データボリュームの生成")
	if serverConfig.Spec.Storage != nil {
		for i, disk := range *serverConfig.Spec.Storage {
			disk.ApiVersion = "v1"
			disk.Kind = "Volume"

			diskID := api.VolumeID(disk)
			if len(diskID) > 0 {
				slog.Debug("既存ボリュームを使用", "disk index", i, "volume id", diskID)
				diskVol, err := m.GetVolumeById(diskID)
				if err != nil {
					slog.Error("GetVolumeById()", "err", err)
					return "", err
				}

				// 永続フラグを立てる
				var peersistent bool = true
				diskVol.Spec.Persistent = &peersistent

				slog.Debug("既存ボリュームの情報取得成功", "disk index", i, "volume id", api.VolumeID(*diskVol), "path", diskVol.Spec.Path, "status", diskVol.Status.Status)
				(*serverConfig.Spec.Storage)[i] = *diskVol
				slog.Debug("既存ボリュームの情報設定成功", "disk index", i, "volume id", api.VolumeID(*diskVol), "disk", disk)
				continue
			}

			if disk.Spec.Type != nil && *disk.Spec.Type == "qcow2" {
				slog.Debug("qcow2ボリュームを作成", "disk index", i)
				assignNodeNameIfUnset(&disk.Metadata, assignedNodeName)
				diskVol, err := m.CreateNewVolumeWithWait(disk)
				if err != nil {
					slog.Error("CreateNewVolumeWithWait()", "err", err)
					return "", err
				}
				(*serverConfig.Spec.Storage)[i] = diskVol
				slog.Debug("データボリューム 作成成功", "disk index", i, "volume id", api.VolumeID(diskVol))
			}
			if disk.Spec.Type != nil && *disk.Spec.Type == "lvm" {
				slog.Debug("lvmボリュームを作成", "disk index", i)
				assignNodeNameIfUnset(&disk.Metadata, assignedNodeName)
				diskVol, err := m.CreateNewVolumeWithWait(disk)
				if err != nil {
					slog.Error("CreateNewVolumeWithWait()", "err", err)
					return "", err
				}
				(*serverConfig.Spec.Storage)[i] = diskVol
				slog.Debug("データボリューム 作成成功", "disk index", i, "volume id", api.VolumeID(diskVol))
			}
		}
	}

	fmt.Println("=== データボリュームの情報確認2 ===", "server Id", api.ServerID(serverConfig))
	data3, err := json.MarshalIndent(serverConfig, "", "  ")
	if err != nil {
		slog.Error("json.MarshalIndent()", "err", err)
	} else {
		fmt.Println("サーバー情報(serverConfig): ", string(data3))
	}

	// データボリュームのIDをサーバーに設定
	err = m.Db.UpdateServer(api.ServerID(serverConfig), serverConfig)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}

	slog.Debug("ハイパーバイザーのリソース確保")
	//var virtSpec virt.ServerSpec
	virtSpec.UUID = *serverConfig.Metadata.Uuid
	if strings.TrimSpace(serverConfig.Metadata.Name) != "" {
		virtSpec.Name = strings.TrimSpace(serverConfig.Metadata.Name) + "-" + api.ServerID(serverConfig) // VMを一意に識別する
	} else {
		virtSpec.Name = "vm-" + api.ServerID(serverConfig)
	}
	// サーバーのVM名前をセットし、今後の操作のためにDBを更新する必要がある
	serverConfig.Metadata.InstanceName = util.StringPtr(virtSpec.Name)

	// CPUとメモリの設定
	slog.Debug("割り当てるCPU数とメモリ量を設定")
	if serverConfig.Spec.Cpu != nil {
		virtSpec.CountVCPU = uint(*serverConfig.Spec.Cpu)
	} else {
		virtSpec.CountVCPU = 2 // デフォルト2
	}

	if serverConfig.Spec.Memory != nil {
		mem := uint(*serverConfig.Spec.Memory) * 1024 //MiB
		virtSpec.RAM = mem
	} else {
		mem := uint(2048 * 1024) // MiB デフォルト2048MB
		virtSpec.RAM = mem
	}
	virtSpec.Machine = "pc-q35-4.2"

	slog.Debug("ボリュームの設定が無いときはqcow2をデフォルトとする1")
	if bootVolDefined.Spec.Type == nil {
		bootVolDefined.Spec.Type = util.StringPtr("qcow2")
	}
	slog.Debug("ボリュームの設定が無いときはqcow2をデフォルトとする2", "boot volume ptr", bootVolDefined)

	path := "/var/lib/marmot/isos/" + api.ServerID(serverConfig)

	var password string
	var sshKey string
	var username string

	if serverConfig.Spec.Auth != nil {
		if serverConfig.Spec.Auth.RootPassword != nil {
			password = *serverConfig.Spec.Auth.RootPassword
		}
		if serverConfig.Spec.Auth.User != nil {
			username = *serverConfig.Spec.Auth.User
		}
		if serverConfig.Spec.Auth.Url != nil {
			keys, err := FetchPublicKeys(*serverConfig.Spec.Auth.Url)
			if err != nil {
				slog.Error("公開鍵の取得に失敗", "url", *serverConfig.Spec.Auth.Url, "err", err)
				return "", err
			}
			sshKey = strings.Join(keys, "\n")
		} else if serverConfig.Spec.Auth.PublicKey != nil {
			sshKey = *serverConfig.Spec.Auth.PublicKey
		}
	}

	isoPath, err := GenerateCloudInitISO(path, password, sshKey, username)
	if err != nil {
		slog.Error("GenerateCloudInitISO()", "err", err)
		return "", err
	}

	switch {
	case *bootVolDefined.Spec.Type == "qcow2":
		if bootVolDefined.Spec.Path == nil || strings.TrimSpace(*bootVolDefined.Spec.Path) == "" {
			return "", fmt.Errorf("boot volume path is required for qcow2")
		}
		virtSpec.DiskSpecs = []virt.DiskSpec{
			{
				Dev:  "vda",
				Src:  *bootVolDefined.Spec.Path,
				Bus:  3,
				Type: *bootVolDefined.Spec.Type,
			},
			{
				Dev:  "sr0",   // CDドライブ
				Src:  isoPath, // ISO ファイルパス
				Bus:  5,       // PCI バス 5
				Type: "iso",   // タイプは iso
			},
		}
	case *bootVolDefined.Spec.Type == "lvm":
		// ＊＊＊　パスは createNewVolume で設定されるべき　＊＊＊
		if bootVolDefined.Spec.VolumeGroup == nil || strings.TrimSpace(*bootVolDefined.Spec.VolumeGroup) == "" {
			return "", fmt.Errorf("boot volume volumeGroup is required for lvm")
		}
		if bootVolDefined.Spec.LogicalVolume == nil || strings.TrimSpace(*bootVolDefined.Spec.LogicalVolume) == "" {
			return "", fmt.Errorf("boot volume logicalVolume is required for lvm")
		}
		lvPath := fmt.Sprintf("/dev/%s/%s", *bootVolDefined.Spec.VolumeGroup, *bootVolDefined.Spec.LogicalVolume)
		virtSpec.DiskSpecs = []virt.DiskSpec{
			{
				Dev:  "vda",
				Src:  lvPath,
				Bus:  3,
				Type: "raw",
			},
			{
				Dev:  "sr0",   // CDドライブ
				Src:  isoPath, // ISO ファイルパス
				Bus:  5,       // PCI バス 5
				Type: "iso",   // タイプは iso
			},
		}
	default:
		slog.Error("CreateServer()", "unsupported volume type", *bootVolDefined.Spec.Type)
		return "", fmt.Errorf("unsupported volume type: %s", *bootVolDefined.Spec.Type)
	}

	// データディスクの設定
	if serverConfig.Spec.Storage != nil {
		for i, disk := range *serverConfig.Spec.Storage {
			if disk.Spec.Kind == nil {
				disk.Spec.Kind = util.StringPtr("data")
			}
			if disk.Spec.Type == nil {
				disk.Spec.Type = util.StringPtr("qcow2")
			}
			switch {
			case *disk.Spec.Type == "qcow2":
				if disk.Spec.Path == nil || strings.TrimSpace(*disk.Spec.Path) == "" {
					return "", fmt.Errorf("storage[%d] path is required for qcow2", i)
				}
				ds := virt.DiskSpec{
					Dev:  fmt.Sprintf("vd%c", 'b'+i),
					Src:  *disk.Spec.Path,
					Bus:  uint(11 + i),
					Type: "qcow2",
				}
				virtSpec.DiskSpecs = append(virtSpec.DiskSpecs, ds)
			case *disk.Spec.Type == "lvm":
				ds := virt.DiskSpec{Dev: fmt.Sprintf("vd%c", 'b'+i), Bus: uint(11 + i)}
				isISCSIDataDisk := disk.Spec.Iscsi != nil && *disk.Spec.Iscsi
				if isISCSIDataDisk {
					targetName, host, port, initiator, err := m.resolveISCSIDiskAttachment(assignedNodeName, disk)
					if err != nil {
						return "", err
					}
					ds.Type = "iscsi"
					ds.ISCSITarget = targetName
					ds.ISCSIHost = host
					ds.ISCSIPort = port
					ds.ISCSIInitiator = initiator
				} else {
					if disk.Spec.Path == nil || strings.TrimSpace(*disk.Spec.Path) == "" {
						return "", fmt.Errorf("storage[%d] path is required for lvm", i)
					}
					ds.Src = *disk.Spec.Path
					ds.Type = "raw"
				}
				virtSpec.DiskSpecs = append(virtSpec.DiskSpecs, ds)
			}
		}
	}

	//channelFile := "org.qemu.guest_agent.0"
	//channelPath, err := util.CreateChannelDir(virtSpec.UUID)

	virtSpec.ChannelSpecs = []virt.ChannelSpec{
		//{"unix", channelPath + "/" + channelFile, channelFile, "channel0", 1},
		{Type: "spicevmc", Path: "", Name: "com.redhat.spice.0", Alias: "channel1", Port: 2},
	}
	virtSpec.Clocks = []virt.ClockSpec{
		{Name: "rtc", TickPolicy: "catchup", Present: ""},
		{Name: "pit", TickPolicy: "delay", Present: ""},
		{Name: "hpet", TickPolicy: "", Present: "no"},
	}

	// イメージのOSメタデータを取得してvirtSpecに設定
	if bootVol.Spec.OsVariant != nil {
		img, imgErr := resolveImageTemplateByVolumeNode(m, bootVol)
		if imgErr == nil {
			if img.Spec.OsName != nil {
				virtSpec.OsName = *img.Spec.OsName
			}
			if img.Spec.OsVersion != nil {
				virtSpec.OsVersion = *img.Spec.OsVersion
			}
		}
	}

	// VM 起動直前に、VXLAN ネットワークの OVS ブリッジ存在を再確認する。
	// ネットワークコントローラーとのタイミング競合でブリッジがまだ見えない場合に備える。
	if err := m.ensureVxlanBridgesForServer(serverConfig); err != nil {
		slog.Error("ensureVxlanBridgesForServer()", "err", err)
		return "", err
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

	consolePath, err := l.DefineAndStartVM(*dom)
	if err != nil && isLibvirtNetworkNotFoundError(err) {
		slog.Warn("network source failed; retrying with bridge fallback", "err", err)
		fallbackApplied := false
		for i := range virtSpec.NetSpecs {
			ns := &virtSpec.NetSpecs[i]
			if strings.TrimSpace(ns.Network) == "" {
				continue
			}
			vnet, lookupErr := m.Db.GetVirtualNetworkByName(ns.Network)
			if lookupErr != nil {
				slog.Warn("bridge fallback skipped: virtual network lookup failed", "network", ns.Network, "err", lookupErr)
				continue
			}
			if vnet.Spec.BridgeName == nil || strings.TrimSpace(*vnet.Spec.BridgeName) == "" {
				continue
			}
			ns.Bridge = strings.TrimSpace(*vnet.Spec.BridgeName)
			ns.Network = ""
			fallbackApplied = true
		}
		if fallbackApplied {
			dom = virt.CreateDomainXML(virtSpec)
			if xml2, marshalErr := dom.Marshal(); marshalErr == nil {
				fmt.Println("Generated", "libvirt XML (fallback):\n", string(xml2))
			}
			consolePath, err = l.DefineAndStartVM(*dom)
		}
	}
	if err != nil && isLibvirtBridgeDeviceMissingError(err) {
		slog.Warn("bridge device not ready; retrying after ensuring vxlan bridges", "err", err)
		if ensureErr := m.ensureVxlanBridgesForServer(serverConfig); ensureErr != nil {
			slog.Error("ensureVxlanBridgesForServer() on retry", "err", ensureErr)
		} else {
			consolePath, err = l.DefineAndStartVM(*dom)
		}
	}
	if err != nil {
		slog.Error("DefineAndStartVM()", "err", err) // ここで No such file or directory エラーになる
		return "", err
	}

	// ステータスを利用可能に更新、更新日時もセット
	serverConfig.Status.StatusCode = db.SERVER_RUNNING
	serverConfig.Status.Status = util.StringPtr(db.ServerStatus[serverConfig.Status.StatusCode])
	serverConfig.Status.Console = util.StringPtr(consolePath)
	serverConfig.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	err = m.Db.UpdateServer(api.ServerID(serverConfig), serverConfig)
	if err != nil {
		slog.Error("UpdateServer()", "err", err)
		return "", err
	}

	return id, nil
}

func isLibvirtNetworkNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "network not found") || strings.Contains(msg, "no network with matching name")
}

func isLibvirtBridgeDeviceMissingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cannot get interface mtu") || strings.Contains(msg, "no such device")
}

func isVxlanOverlayNetwork(vnet api.VirtualNetwork) bool {
	if vnet.Spec.OverlayMode == nil {
		return false
	}
	return strings.EqualFold(string(*vnet.Spec.OverlayMode), "vxlan")
}

func (m *Marmot) ensureVxlanBridgesForServer(serverConfig api.Server) error {
	if serverConfig.Spec.NetworkInterface == nil {
		return nil
	}

	fabric := networkfabric.NewOVSFabric()
	for _, nic := range *serverConfig.Spec.NetworkInterface {
		networkName := strings.TrimSpace(nic.Networkname)
		if networkName == "" {
			continue
		}

		vnet, err := m.Db.GetVirtualNetworkByName(networkName)
		if err != nil {
			return fmt.Errorf("failed to lookup virtual network %s: %w", networkName, err)
		}
		if !isVxlanOverlayNetwork(vnet) {
			continue
		}
		if vnet.Spec.BridgeName == nil || strings.TrimSpace(*vnet.Spec.BridgeName) == "" {
			return fmt.Errorf("vxlan network %s has no bridgeName", networkName)
		}

		if err := fabric.EnsureBridge(&vnet); err != nil {
			return fmt.Errorf("failed to ensure bridge for network %s: %w", networkName, err)
		}
	}

	return nil
}

// サーバーの停止 コントローラーから呼び出される
func (m *Marmot) StopServerManage(id string) error {
	slog.Debug("===StopServerManage() is called===", "id", id)
	sv, err := m.Db.GetServerById(id)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return err
	}

	if sv.Metadata.InstanceName == nil {
		return fmt.Errorf("server %s has no instance name, cannot stop", id)
	}

	l, err := virt.NewLibVirtEp("qemu:///system")
	if err != nil {
		slog.Error("NewLibVirtEp()", "err", err)
		return err
	}
	defer l.Close()

	if err = l.StopDomain(*sv.Metadata.InstanceName); err != nil {
		slog.Error("StopDomain()", "err", err)
		return err
	}

	sv.Status.StatusCode = db.SERVER_STOPPED
	sv.Status.Status = util.StringPtr(db.ServerStatus[sv.Status.StatusCode])
	sv.Status.Console = util.StringPtr("")
	sv.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	if err = m.Db.UpdateServer(api.ServerID(sv), sv); err != nil {
		slog.Error("UpdateServer()", "err", err)
		return err
	}

	return nil
}

// サーバーの起動 コントローラーから呼び出される
func (m *Marmot) StartServerManage(id string) error {
	slog.Debug("===StartServerManage() is called===", "id", id)
	sv, err := m.Db.GetServerById(id)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return err
	}

	if sv.Metadata.InstanceName == nil {
		return fmt.Errorf("server %s has no instance name, cannot start", id)
	}

	l, err := virt.NewLibVirtEp("qemu:///system")
	if err != nil {
		slog.Error("NewLibVirtEp()", "err", err)
		return err
	}
	defer l.Close()

	if err = l.StartDomain(*sv.Metadata.InstanceName); err != nil {
		slog.Error("StartDomain()", "err", err)
		return err
	}

	dom, err := l.Com.LookupDomainByName(*sv.Metadata.InstanceName)
	if err != nil {
		slog.Error("LookupDomainByName()", "err", err)
		return err
	}
	defer dom.Free()

	consolePath, err := virt.GetDomainConsolePath(dom)
	if err != nil {
		slog.Error("GetDomainConsolePath()", "err", err)
		return err
	}

	sv.Status.StatusCode = db.SERVER_RUNNING
	sv.Status.Status = util.StringPtr(db.ServerStatus[sv.Status.StatusCode])
	sv.Status.Console = util.StringPtr(consolePath)
	sv.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	if err = m.Db.UpdateServer(api.ServerID(sv), sv); err != nil {
		slog.Error("UpdateServer()", "err", err)
		return err
	}

	return nil
}

// サーバーの削除 コントローラーから呼び出される
func (m *Marmot) DeleteServerByIdManage(id string) error {
	slog.Debug("===DeleteServerById is called===", "id", id)
	sv, err := m.Db.GetServerById(id)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return err
	}

	serverName := sv.Metadata.Name

	if sv.Metadata.InstanceName != nil {
		// サーバーの削除
		l, err := virt.NewLibVirtEp("qemu:///system")
		if err != nil {
			slog.Error("NewLibVirtEp()", "err", err)
			return err
		}
		defer l.Close()

		// この下で異常動作が起きている
		slog.Debug("DeleteServerById()", "deleting domain", *sv.Metadata.InstanceName)

		if err = l.DeleteDomain(*sv.Metadata.InstanceName); err != nil {
			// ドメインが存在しない場合はスキップしたいが、区別が難しいので意図的にスキップする
			//if *sv.Status != db.SERVER_PROVISIONING {
			//	slog.Error("DeleteDomain()", "err", err)
			//	return err
			//}
			slog.Debug("DeleteServerById()", "server is in PROVISIONING state, skipping domain deletion", serverName)
			// return nil 戻さず、削除処理を続行する
		}
	}

	// ブートボリュームの削除タイムスタンプのセット
	if sv.Spec.BootVolume != nil && strings.TrimSpace(api.VolumeID(*sv.Spec.BootVolume)) != "" {
		m.Db.SetVolumeDeletionTimestamp(api.VolumeID(*sv.Spec.BootVolume))
	} else {
		slog.Warn("DeleteServerByIdManage() boot volume is missing, skipping deletion timestamp", "serverId", id, "serverName", serverName)
	}

	// データボリュームの削除タイムスタンプのセット
	if sv.Spec.Storage != nil {
		slog.Debug("アタッチされているボリューム削除のため Deletion Timestamp をセット", "ボリューム数", len(*sv.Spec.Storage))
		for i, vol := range *sv.Spec.Storage {
			volID := api.VolumeID(vol)
			slog.Debug("DeleteServerById()", "index", i, "deleting volume id", volID)
			if vol.Spec.Persistent != nil && *vol.Spec.Persistent {
				slog.Debug("DeleteServerById()", "skipping persistent volume", volID)
				continue
			}
			if strings.TrimSpace(volID) == "" {
				slog.Warn("DeleteServerByIdManage() attached volume id is empty, skipping", "serverId", id, "index", i)
				continue
			}
			m.Db.SetVolumeDeletionTimestamp(volID)
		}
	}

	return nil
}

// サーバーのリストを取得 ラップ関数  コントローラーから呼び出される
func (m *Marmot) GetServersManage() (api.Servers, error) {
	slog.Debug("GetServersManage is called===", "", "")
	serverSpec, err := m.Db.GetServers()
	if err != nil {
		slog.Error("GetServersManage()", "err", err)
		return nil, err
	}
	return serverSpec, nil
}

// サーバーの詳細を取得
func (m *Marmot) GetServerManage(id string) (api.Server, error) {
	slog.Debug("===GetServerById is called===", "id", id)
	serverSpec, err := m.Db.GetServerById(id)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return api.Server{}, err
	}

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

// ボリュームリクエストに基づいて、新しいボリュームを作成する
func (m *Marmot) CreateNewVolumeWithWait(volReq api.Volume) (api.Volume, error) {
	slog.Debug("===CreateNewVolume is called===", "volume request", volReq)

	vol, err := m.Db.CreateVolumeOnDB2(volReq)
	if err != nil {
		slog.Error("CreateVolumeOnDB2()", "err", err)
		// サーバーとボリュームのステータスをエラーに更新する処理を追加するべき

		return api.Volume{}, err
	}

	volDefined, err := m.CreateNewVolume(api.VolumeID(*vol))
	if err != nil {
		slog.Error("CreateNewVolume()", "err", err)
		//
		return api.Volume{}, err
	}

	for {
		vol, err := m.GetVolumeById(api.VolumeID(*vol))
		if err != nil {
			slog.Error("GetVolumeById()", "err", err)
			// サーバーとボリュームのステータスをエラーに更新する処理を追加するべき
			return api.Volume{}, err
		}
		slog.Debug("ブートボリュームのステータス確認ループ", "volume id", api.VolumeID(*vol), "status", vol.Status.Status)
		if vol.Status.StatusCode == db.VOLUME_AVAILABLE {
			slog.Debug("ブートボリュームのステータスがAVAILABLEになった", "volume id", api.VolumeID(*vol))
			break
		}
		if vol.Status.StatusCode == db.VOLUME_ERROR {
			msg := "volume provisioning failed"
			if vol.Status.Message != nil && len(*vol.Status.Message) > 0 {
				msg = *vol.Status.Message
			}
			return api.Volume{}, fmt.Errorf("volume %s is in error state: %s", api.VolumeID(*vol), msg)
		}
		time.Sleep(1 * time.Second)
	}

	return *volDefined, nil
}

// サーバーから起動イメージの作成
func (m *Marmot) MakeImageEntryFromRunningVM(serverId, name string, image api.Image) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), CurrentConfig().ImageCreateFromVMTimeout())
	defer cancel()
	return m.MakeImageEntryFromRunningVMWithContext(ctx, serverId, name, image)
}

// サーバーから起動イメージの作成
func (m *Marmot) MakeImageEntryFromRunningVMWithContext(ctx context.Context, serverId, name string, image api.Image) (string, error) {
	slog.Debug("===MakeImageEntryFromRunningVM is called===", "server id", serverId)
	if ctx == nil {
		ctx = context.Background()
	}
	operationTimeout := contextTimeoutHint(ctx)
	markFailed := func(err error) error {
		err = wrapDeadlineExceeded(err, "実行中 VM からのイメージ作成", operationTimeout)
		return m.markImageCreationFailed(image, err)
	}

	if err := ctx.Err(); err != nil {
		return "", markFailed(err)
	}

	// サーバーの情報を取得
	serverSpec, err := m.Db.GetServerById(serverId)
	if err != nil {
		slog.Error("GetServerById()", "err", err, "server id", serverId)
		return "", markFailed(err)
	}

	// ブートボリュームのIDを取得
	//slog.Debug("MakeImageEntryFromRunningVM()", "boot volume id", serverSpec.Spec.BootVolume.Id)

	// ブートボリュームの情報を取得
	bootVol, err := m.GetVolumeById(api.VolumeID(*serverSpec.Spec.BootVolume))
	if err != nil {
		slog.Error("GetVolumeById()", "err", err, "volume id", api.VolumeID(*serverSpec.Spec.BootVolume))
		return "", markFailed(err)
	}
	//slog.Debug("MakeImageEntryFromRunningVM()", "boot volume", bootVol.Spec.Path)

	// イメージIDの取得、名前チェック
	//image, err := m.Db.MakeImageEntryFromRunningVM(serverId, name)
	//if err != nil {
	//	slog.Error("MakeImageEntryFromRunningVM()", "err", err, "server id", serverId)
	//	return "", err
	//}

	// 仮想マシンの一時停止
	if err := m.Virt.StopDomain(*serverSpec.Metadata.InstanceName); err != nil {
		slog.Error("StopDomain()", "err", err)
		//return "", err
	}

	if bootVol.Spec.Type != nil && *bootVol.Spec.Type == "qcow2" {
		// qcow2ファイルのコピー
		slog.Debug("qcow2ファイルのコピー", "source path", *bootVol.Spec.Path, "destination path", *image.Spec.Qcow2Path)
		if bootVol.Spec.Path != nil {
			// 物理的なボリュームのコピー
			if err := qcow.CopyQcowWithContext(ctx, *bootVol.Spec.Path, *image.Spec.Qcow2Path); err != nil {
				slog.Error("qcow.CopyQcow()", "err", err)
				return "", markFailed(err)
			}
		}
	} else if bootVol.Spec.Type != nil && *bootVol.Spec.Type == "lvm" {
		// LVMのスナップショット作成とコピー
		slog.Debug("LVMのスナップショット作成とコピー", "volume group", *bootVol.Spec.VolumeGroup, "logical volume", *bootVol.Spec.LogicalVolume, "destination logical volume", *image.Spec.LogicalVolume)
		if bootVol.Spec.VolumeGroup != nil && bootVol.Spec.LogicalVolume != nil {
			// スナップショットを取って、コピーする方向に変更が必要。しかし、実装は後にする
			// 同じサイズのボリュームを作成して、dd でコピーを作成する。
			size := uint64(*image.Spec.Size * 1024 * 1024 * 1024) // スナップショットサイズは16GB固定
			//if err := lvm.CreateSnapshot(*bootVol.Spec.VolumeGroup, *bootVol.Spec.LogicalVolume, *image.Spec.LogicalVolume, size); err != nil {
			//	slog.Error("lvm.CreateSnapshot()", "err", err)
			//}

			if err := lvm.CopyLogicalVoulumeWithContext(ctx, *bootVol.Spec.VolumeGroup, *bootVol.Spec.LogicalVolume, CurrentConfig().OSVolumeGroup, *image.Spec.LogicalVolume, size); err != nil {
				slog.Error("lvm.CopyLogicalVoulume()", "err", err)
				return "", markFailed(err)
			}
		}
	} else {
		err := fmt.Errorf("unsupported boot volume type for image creation")
		slog.Error("MakeImageEntryFromRunningVM()", "err", err, "server id", serverId)
		return "", markFailed(err)
	}

	// 仮想マシンの再開
	if err := m.Virt.StartDomain(*serverSpec.Metadata.InstanceName); err != nil {
		slog.Error("StartDomain()", "err", err)
		return "", markFailed(err)
	}

	// イメージ情報の登録
	imageID := image.Metadata.Id
	if imageID == "" {
		err := fmt.Errorf("image metadata.id is empty")
		slog.Error("MakeImageEntryFromRunningVMWithContext()", "err", err, "serverId", serverId, "imageName", name)
		return "", markFailed(err)
	}
	m.Db.UpdateImageStatus(imageID, db.IMAGE_AVAILABLE)

	return imageID, nil
}
