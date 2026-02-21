package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"net/netip"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	etcd "go.etcd.io/etcd/client/v3"
)

/*
IPアドレス管理（IPAM）パッケージは、仮想ネットワーク内のIPアドレスの割り当てと管理を担当します。以下は、IPAMパッケージの基本的な構造と機能の概要です。

1. IPアドレスプールの管理:
   - IPアドレスの範囲を定義し、利用可能なIPアドレスのリストを管理します。
   - IPアドレスの割り当てと解放の機能を提供します。

2. IPアドレスの割り当て:
   - 仮想マシンやコンテナに対してIPアドレスを割り当てる機能を提供します。
   - 割り当てられたIPアドレスの追跡と管理を行います。

3. IPアドレスの解放:
   - 使用されなくなったIPアドレスを解放し、再利用可能な状態に戻す機能を提供します。

4. IPアドレスのクエリ:
   - 現在割り当てられているIPアドレスのリストを取得する機能を提供します。

5. エラーハンドリング:
   - IPアドレスの割り当てや解放に失敗した場合のエラーハンドリングを実装します。

このIPAMパッケージは、仮想ネットワークのIPアドレス管理を効率的に行うための基本的な機能を提供し、仮想マシンやコンテナのネットワーク設定をサポートします。
*/

// IPネットワークアドレス、サブネットマスク、ゲートウェイなどを作成して、etcdに永続化する
// 戻り値は、ネットワークIDなどの識別子
func (d *Database) CreateIpNetwork(spec *api.IPNetwork) (string, error) {
	slog.Debug("CreateIpNetwork()", "spec", spec)

	if spec.AddressMaskLen == nil {
		slog.Error("CreateIpNetwork()", "err", "AddressMaskLen is required", "spec", spec)
		return "", fmt.Errorf("AddressMaskLen is required")
	}
	// IPアドレスとネットマスクが指定されていることを確認
	prefix, err := netip.ParsePrefix(*spec.AddressMaskLen)
	if err != nil {
		slog.Error("CreateIpNetwork()", "err", err, "spec", spec)
		return "", fmt.Errorf("invalid AddressMaskLen: %v", err)
	}

	// 同一ネットワークアドレスが存在しないことを確認
	networks, err := d.GetIpNetworks()
	if err != nil {
		slog.Error("CreateIpNetwork()", "err", err)
		return "", err
	}
	for _, net := range networks {
		if net.AddressMaskLen != nil && *net.AddressMaskLen == *spec.AddressMaskLen {
			slog.Error("CreateIpNetwork()", "err", "Network with the same AddressMaskLen already exists", "spec", spec)
			return "", fmt.Errorf("Network with the same AddressMaskLen already exists")
		}
		// ネットワークアドレスが重複していないことを確認
		if net.AddressMaskLen != nil {
			existingPrefix, err := netip.ParsePrefix(*net.AddressMaskLen)
			if err != nil {
				slog.Error("CreateIpNetwork()", "err", fmt.Errorf("invalid existing AddressMaskLen: %v", err), "existingSpec", net)
				continue
			}
			if prefix.Overlaps(existingPrefix) {
				slog.Error("CreateIpNetwork()", "err", "Network overlaps with an existing network", "spec", spec, "existingSpec", net)
				return "", fmt.Errorf("Network overlaps with an existing network")
			}
		}
	}

	//一意なIDを発行
	var id string
	var key string
	for {
		var tempNet api.IPNetwork
		id = uuid.New().String()[:5]
		key = IPNetworkPrefix + "/" + id
		_, err := d.GetJSON(key, &tempNet)
		if err == ErrNotFound {
			break
		} else if err != nil {
			slog.Error("CreateIpNetwork()", "err", err)
			return "", err
		}
	}

	// IPネットワークの情報をコピーして、開始アドレスと終了アドレスを設定する
	net, err := util.DeepCopy(spec)
	if err != nil {
		slog.Error("CreateIpNetwork()", "err", err)
		return "", err
	}

	// IPアドレスの開始と終了アドレスを設定する
	networkAddr := prefix.Masked().Addr()
	addr := networkAddr.Next()
	net.StartAddress = util.StringPtr(addr.String())
	addr = addIP(addr, int64(math.Pow(2, float64(int(prefix.Addr().BitLen()-prefix.Bits()))))-3) // ブロードキャストアドレスとゲートウェイを考慮して-3
	net.EndAddress = util.StringPtr(addr.String())

	// etcdに永続化する
	net.Id = id
	err = d.PutJSON(key, net)
	if err != nil {
		slog.Error("CreateIpNetwork()", "err", err)
		return "", err
	}

	return id, nil
}

// IPネットワークアドレスを無指定で、自動生成して永続化する
// 戻り値は、ネットワークIDなどの識別子
func (d *Database) CreateAnyIpNetwork() (string, error) {
	return "", nil
}

func (d *Database) GetIpNetworks() ([]api.IPNetwork, error) {
	var networks []api.IPNetwork
	var err error
	var resp *etcd.GetResponse

	slog.Debug("GetIpNetworks()", "key-prefix", IPNetworkPrefix)
	resp, err = d.GetByPrefix(IPNetworkPrefix)
	if err == ErrNotFound {
		slog.Debug("no networks found", "key-prefix", IPNetworkPrefix)
		return networks, nil
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", IPNetworkPrefix)
		return networks, err
	}

	for _, ev := range resp.Kvs {
		var net api.IPNetwork
		err := json.Unmarshal(ev.Value, &net)
		if err != nil {
			slog.Error("failed to unmarshal network data", "err", err, "key", string(ev.Key))
			continue
		}
		networks = append(networks, net)
	}

	return networks, nil
}

func (d *Database) GetIpNetworkById(id string) (*api.IPNetwork, error) {
	key := IPNetworkPrefix + "/" + id
	var net api.IPNetwork
	_, err := d.GetJSON(key, &net)
	if err != nil {
		slog.Error("GetIpNetworkById()", "err", err, "id", id)
		return nil, err
	}
	return &net, nil
}

// IPネットワークアドレスを削除する
// IPネットワークに割当られたIPがある場合は、削除できないようにする
func (d *Database) DeleteIpNetworkById(id string) error {
	// 削除するネットワークに割り当てられたIPアドレスが存在するか確認する
	ips, err := d.GetAllocatedIPs(id)
	if err != nil {
		slog.Error("GetAllocatedIPs() failed", "err", err, "netId", id)
		return err
	}
	if len(ips) > 0 {
		slog.Error("DeleteIpNetworkById() failed", "err", fmt.Errorf("cannot delete network with allocated IPs"), "netId", id, "allocatedIPsCount", len(ips))
		return fmt.Errorf("cannot delete network with allocated IPs")
	}	

	key := IPNetworkPrefix + "/" + id
	return d.DeleteJSON(key)
}

// IPネットワークのIDをセットして、そのネットワークからIPアドレスを一つ割り当てる。
// 取得したIPアドレスは、仮想マシンやコンテナのネットワーク設定に使用される。
// 渡したホストIDは、このホストによって使用中であることを示すために使用される。
func (d *Database) AllocateIP(netId, hostId string) (string, error) {
	net, err := d.GetIpNetworkById(netId)
	if err == nil || err.Error() == "not found" {
		// NOP
	} else if err != nil {
		slog.Error("AllocateIP()", "err", err, "netId", netId)
		return "", err
	}

	prefix, err := netip.ParsePrefix(*net.AddressMaskLen)
	if err != nil {
		slog.Error("AllocateIP()", "err", err, "netId", netId)
		return "", err
	}
	networkAddr := prefix.Masked().Addr()
	// 最初のホストアドレス (.1) から開始
	addr := networkAddr.Next()

	// 割り当てられているIPアドレスと比較して、未割り当てのIPアドレスを見つける
	for {
		nextAddr := addr.Next()
		if !prefix.Contains(nextAddr) {
			slog.Error("AllocateIP()", "err", "no available IP addresses in the network", "netId", netId)
			return "", fmt.Errorf("no available IP addresses in the network")
		}
		// 次へ進む
		addr = nextAddr

		// 異常値チェック
		if !addr.IsValid() {
			slog.Error("AllocateIP()", "err", "IP address is not valid", "netId", netId)
			return "", fmt.Errorf("no available IP addresses in the network")
		}

		// ブロードキャストアドレスは使わない
		nextAddr2 := addr.Next()
		if !prefix.Contains(nextAddr2) {
			slog.Error("AllocateIP()", "err", "no available IP addresses in the network", "netId", netId)
			return "", fmt.Errorf("no available IP addresses in the network")
		}

		// 一致するものが無かったら、そのIPアドレスを割り当てる
		found, err := d.CheckIPaddrInUse(netId, addr.String())
		if err != nil {
			slog.Error("AllocateIP()", "err", err, "netId", netId, "candidateIP", addr.String())
			return "", err
		}
		if !found {
			slog.Debug("割り当てられたIPアドレス", "IP	", addr.String())
			d.SetIPaddrInUse(netId, addr.String(), hostId)
			return addr.String(), nil
		}
	}

	// ここに到達した場合、利用可能なIPアドレスがないことを意味する
	return "", fmt.Errorf("no available IP addresses in the network")
}

// IPアドレスを解放する
func (d *Database) ReleaseIP(netId, ip string) error {
	net, err := d.GetIpNetworkById(netId)
	if err != nil {
		slog.Error("GetIpNetworkById()", "err", err, "netId", netId)
		return err
	}

	key := IPAddressPrefix + "/" + *net.AddressMaskLen + "/" + ip
	return d.DeleteJSON(key)
}

func (d *Database) CheckIPaddrInUse(netId, ip string) (bool, error) {
	net, err := d.GetIpNetworkById(netId)
	if err != nil {
		slog.Error("GetIpNetworkById()", "err", err, "netId", netId)
		return false, err
	}

	key := IPAddressPrefix + "/" + *net.AddressMaskLen + "/" + ip
	var rec api.IPAddress
	if _, err = d.GetJSON(key, &rec); err == ErrNotFound {
		return false, nil
	} else if err != nil {
		slog.Error("CheckIPaddrInUse()", "err", err, "netId", netId, "ip", ip)
		return false, err
	}
	return true, nil
}

// ネットワークIDをセットして、そのネットワークから割り当てられたIPアドレスを使用中としてマークする
func (d *Database) SetIPaddrInUse(netId, ip, hostId string) error {
	var rec api.IPAddress
	rec.HostId = util.StringPtr(hostId)
	rec.IPAddress = util.StringPtr(ip)
	rec.NetworkId = util.StringPtr(netId)

	net, err := d.GetIpNetworkById(netId)
	if err != nil {
		slog.Error("GetIpNetworkById()", "err", err, "netId", netId)
		return err
	}

	key := IPAddressPrefix + "/" + *net.AddressMaskLen + "/" + ip

	// デバック
	fmt.Println("key=", key)
	byteJson, err := json.MarshalIndent(rec, "", "    ")
	if err != nil {
		slog.Error("SetIPaddrInUse()", "err", err, "rec", rec)
	}
	fmt.Println("rec=", string(byteJson))

	return d.PutJSON(key, rec)
}

func (d *Database) GetAllocatedIPs(netId string) ([]api.IPAddress, error) {
	net, err := d.GetIpNetworkById(netId)
	if err != nil {
		slog.Error("GetIpNetworkById()", "err", err, "netId", netId)
		return nil, err
	}

	keyPrefix := IPAddressPrefix + "/" + *net.AddressMaskLen + "/"
	resp, err := d.GetByPrefix(keyPrefix)
	if err == ErrNotFound {
		return []api.IPAddress{}, nil
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", keyPrefix)
		return nil, err
	}

	var allocatedIPs []api.IPAddress
	for _, ev := range resp.Kvs {
		var rec api.IPAddress
		err := json.Unmarshal(ev.Value, &rec)
		if err != nil {
			slog.Error("failed to unmarshal IP address data", "err", err, "key", string(ev.Key))
			continue
		}
		//if rec.NetworkId != nil && *rec.NetworkId == netId {
		allocatedIPs = append(allocatedIPs, rec)
		//}
	}

	return allocatedIPs, nil
}

// IPアドレスに整数を加算する関数
func addIP(ip netip.Addr, delta int64) netip.Addr {
	// IPアドレスを16バイトのバイトスライスに変換
	ipBytes := ip.As16()

	// バイトスライスを big.Int に変換
	val := new(big.Int).SetBytes(ipBytes[:])

	// 加算を実行
	val.Add(val, big.NewInt(delta))

	// 計算結果をバイト配列に戻す
	resultBytes := val.Bytes()

	// 16バイトに満たない場合は左側を0埋めする（IPv6用）
	var newAddr [16]byte
	copy(newAddr[16-len(resultBytes):], resultBytes)

	// netip.Addr に戻す（IPv4の場合は自動的に調整されます）
	newIP := netip.AddrFrom16(newAddr)
	if ip.Is4() {
		return newIP.Unmap() // 元がIPv4ならIPv4形式に戻す
	}
	return newIP
}
