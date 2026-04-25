package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"net/netip"
	"strings"

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
// 戻り値は、ネットワークIDなどの識別子、エラー時でもIDを返す
func (d *Database) CreateIpNetwork(vnetid string, spec *api.IPNetwork) (string, error) {
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
	prefix = prefix.Masked()

	// 既存のネットワークアドレスのリストを取得
	// IDは返す
	networks, err := d.GetIpNetworks(vnetid)
	if err != nil {
		slog.Error("CreateIpNetwork()", "err", err)
		return "", err
	}

	// 同一ネットワークアドレスが存在しないことを確認
	for _, net := range networks {
		if net.AddressMaskLen != nil {
			existingPrefix, err := netip.ParsePrefix(*net.AddressMaskLen)
			if err != nil {
				slog.Error("CreateIpNetwork()", "err", fmt.Errorf("invalid existing AddressMaskLen: %v", err), "existingSpec", net)
				continue
			}
			existingPrefix = existingPrefix.Masked()

			if existingPrefix == prefix {
				return net.Id, fmt.Errorf(ErrAlreadyExists)
			}

			// ネットワークアドレスが重複していないことを確認
			if prefix.Overlaps(existingPrefix) {
				//slog.Error("CreateIpNetwork()", "err", "Network overlaps with an existing network", "spec", spec, "existingSpec", net)
				return net.Id, fmt.Errorf(ErrOverlapsExistingNetwork)
			}
		}
	}

	//一意なIDを発行
	var id string
	var key string
	for {
		var tempNet api.IPNetwork
		id = uuid.New().String()[:5]
		key = NetworkPrefix + "/" + vnetid + "/ip_network/" + id
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
	net.AddressMaskLen = util.StringPtr(prefix.String())

	// IPアドレスの開始と終了アドレスを設定する
	networkAddr := prefix.Addr()
	addr := networkAddr.Next()
	net.Netmasklen = util.IntPtrInt(prefix.Bits())
	net.StartAddress = util.StringPtr(addr.String())
	addr = addIP(addr, int64(math.Pow(2, float64(int(prefix.Addr().BitLen()-prefix.Bits()))))-3) // ブロードキャストアドレスとゲートウェイを考慮して-3
	net.EndAddress = util.StringPtr(addr.String())
	net.NetworkAddress = util.StringPtr(networkAddr.String())
	net.Gateway = util.StringPtr(networkAddr.Next().String()) // ゲートウェイはネットワークアドレスの次のアドレスとする
	netmask, err := PrefixLenToMask(prefix.Bits(), prefix.Addr().Is6())
	if err != nil {
		slog.Error("CreateIpNetwork()", "err", err)
		return "", err
	}
	net.Netmask = util.StringPtr(netmask)

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
func (d *Database) CreateAnyIpNetwork(vnetid string) (string, error) {
	slog.Debug("CreateAnyIpNetwork()", "spec", "")
	for i := 100; i < 200; i++ {
		netadd := fmt.Sprintf("172.16.%d.0/24", i)
		ipNetSpec := &api.IPNetwork{
			AddressMaskLen: util.StringPtr(netadd),
		}
		id, err := d.CreateIpNetwork(vnetid, ipNetSpec)
		if err != nil {
			continue
		} else {
			return id, nil
		}
	}
	slog.Error("CreateAnyIpNetwork()", "err", "Failed to create any IP network after 100 attempts")
	return "", fmt.Errorf("failed to create any IP network after 100 attempts")
}

func (d *Database) GetIpNetworks(vnetid string) ([]api.IPNetwork, error) {
	var networks []api.IPNetwork
	var err error
	var resp *etcd.GetResponse

	key := NetworkPrefix + "/" + vnetid + "/ip_network/"
	slog.Debug("GetIpNetworks()", "key-prefix", key)
	//resp, err = d.GetByPrefix(IPNetworkPrefix + "/" + vnetid)
	resp, err = d.GetByPrefix(key)
	if err == ErrNotFound {
		slog.Debug("no networks found", "key-prefix", key)
		return networks, nil
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", key)
		return networks, err
	}

	for _, ev := range resp.Kvs {
		relKey := strings.TrimPrefix(string(ev.Key), key)
		if relKey == "" || strings.Contains(relKey, "/") {
			// /ip_network/{id} の直下のみを IPNetwork として扱う。
			// /ip_network/{id}/ip_address/... は除外する。
			continue
		}

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

func (d *Database) GetIpNetworkById(vnetid, id string) (*api.IPNetwork, error) {
	//key := IPNetworkPrefix + "/" + id
	key := NetworkPrefix + "/" + vnetid + "/ip_network/" + id
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
func (d *Database) DeleteIpNetworkById(vnetId, ipnetId string) error {
	// 削除するネットワークに割り当てられたIPアドレスが存在するか確認する
	ips, err := d.GetAllocatedIPs(vnetId, ipnetId)
	if err != nil {
		slog.Error("GetAllocatedIPs() failed", "err", err, "vnetId", vnetId, "ipnetId", ipnetId)
		return err
	}
	if len(ips) > 0 {
		slog.Error("DeleteIpNetworkById() failed", "err", fmt.Errorf("cannot delete network with allocated IPs"), "vnetId", vnetId, "ipnetId", ipnetId, "allocatedIPsCount", len(ips))
		return fmt.Errorf("cannot delete network with allocated IPs")
	}

	key := NetworkPrefix + "/" + vnetId + "/ip_network/" + ipnetId
	return d.DeleteJSON(key)
}

// IPネットワークのIDをセットして、そのネットワークからIPアドレスを一つ割り当てる。
// 取得したIPアドレスは、仮想マシンやコンテナのネットワーク設定に使用される。
// 渡したホストIDは、このホストによって使用中であることを示すために使用される。
func (d *Database) AllocateIP(vnetId, ipnetId, hostId string) (string, int, error) {
	net, err := d.GetIpNetworkById(vnetId, ipnetId)
	if err == nil || err.Error() == "not found" {
		// NOP
	} else if err != nil {
		slog.Error("AllocateIP()", "err", err, "vnetId", vnetId, "ipnetId", ipnetId)
		return "", 0, err
	}

	prefix, err := netip.ParsePrefix(*net.AddressMaskLen)
	if err != nil {
		slog.Error("AllocateIP()", "err", err, "vnetId", vnetId, "ipnetId", ipnetId)
		return "", 0, err
	}
	networkAddr := prefix.Masked().Addr()
	// 最初のホストアドレス (.1) から開始
	addr := networkAddr.Next()

	// 割り当てられているIPアドレスと比較して、未割り当てのIPアドレスを見つける
	for {
		nextAddr := addr.Next()
		if !prefix.Contains(nextAddr) {
			slog.Error("AllocateIP()", "err", "no available IP addresses in the network", "vnetId", vnetId, "ipnetId", ipnetId)
			return "", 0, fmt.Errorf("no available IP addresses in the network")
		}
		// 次へ進む
		addr = nextAddr

		// 異常値チェック
		if !addr.IsValid() {
			slog.Error("AllocateIP()", "err", "IP address is not valid", "vnetId", vnetId, "ipnetId", ipnetId)
			return "", 0, fmt.Errorf("no available IP addresses in the network")
		}

		// ブロードキャストアドレスは使わない
		nextAddr2 := addr.Next()
		if !prefix.Contains(nextAddr2) {
			slog.Error("AllocateIP()", "err", "no available IP addresses in the network", "vnetId", vnetId, "ipnetId", ipnetId)
			return "", 0, fmt.Errorf("no available IP addresses in the network")
		}

		// 一致するものが無かったら、そのIPアドレスを割り当てる
		found, err := d.CheckIPaddrInUse(vnetId, ipnetId, addr.String())
		if err != nil {
			slog.Error("AllocateIP()", "err", err, "vnetId", vnetId, "ipnetId", ipnetId, "candidateIP", addr.String())
			return "", 0, err
		}
		if !found {
			slog.Debug("割り当てられたIPアドレス", "IP	", addr.String())
			d.SetIPaddrInUse(vnetId, ipnetId, addr.String(), hostId)
			return addr.String(), prefix.Bits(), nil
		}
	}

	// ここに到達した場合、利用可能なIPアドレスがないことを意味する
	return "", 0, fmt.Errorf("no available IP addresses in the network")
}

// IPアドレスを解放する
func (d *Database) ReleaseIP(vnetId, ipnetId, ip string) error {
	net, err := d.GetIpNetworkById(vnetId, ipnetId)
	if err != nil {
		slog.Error("GetIpNetworkById()", "err", err, "vnetId", vnetId, "ipnetId", ipnetId)
		return err
	}

	key := NetworkPrefix + "/" + vnetId + "/ip_network/" + ipnetId + "/ip_address/" + *net.AddressMaskLen + "/" + ip
	return d.DeleteJSON(key)
}

// IPアドレスが存在するかどうかをチェックする
func (d *Database) CheckIPaddrInUse(vnetId, ipnetId, ip string) (bool, error) {
	net, err := d.GetIpNetworkById(vnetId, ipnetId)
	if err != nil {
		slog.Error("GetIpNetworkById()", "err", err, "vnetId", vnetId, "ipnetId", ipnetId)
		return false, err
	}

	key := NetworkPrefix + "/" + vnetId + "/ip_network/" + ipnetId + "/ip_address/" + *net.AddressMaskLen + "/" + ip
	var rec api.IPAddress
	if _, err = d.GetJSON(key, &rec); err == ErrNotFound {
		return false, nil
	} else if err != nil {
		slog.Error("CheckIPaddrInUse()", "err", err, "vnetId", vnetId, "ipnetId", ipnetId, "ip", ip)
		return false, err
	}
	return true, nil
}

// この仮想ネットワークに関連づいているIPネットワークが使用中かどうかをチェックする
func (d *Database) CheckIPnetInUse(vnetId, ipnetId string) (bool, error) {
	key := NetworkPrefix + "/" + vnetId + "/ip_network/" + ipnetId + "/ip_address/"
	if _, err := d.GetByPrefix(key); err == ErrNotFound {
		return false, nil
	} else if err != nil {
		slog.Error("CheckIPnetInUse()", "err", err, "vnetId", vnetId, "ipnetId", ipnetId)
		return true, err
	}
	return true, nil
}

// ネットワークIDをセットして、そのネットワークから割り当てられたIPアドレスを使用中としてマークする
func (d *Database) SetIPaddrInUse(vnetId, ipnetId, ip, hostId string) error {
	var rec api.IPAddress
	rec.HostId = util.StringPtr(hostId)
	rec.IPAddress = util.StringPtr(ip)
	rec.NetworkId = util.StringPtr(ipnetId)

	net, err := d.GetIpNetworkById(vnetId, ipnetId)
	if err != nil {
		slog.Error("GetIpNetworkById()", "err", err, "vnetId", vnetId, "ipnetId", ipnetId)
		return err
	}
	key := NetworkPrefix + "/" + vnetId + "/ip_network/" + ipnetId + "/ip_address/" + *net.AddressMaskLen + "/" + ip
	//key := IPAddressPrefix + "/" + *net.AddressMaskLen + "/" + ip

	// デバック
	fmt.Println("key=", key)
	byteJson, err := json.MarshalIndent(rec, "", "    ")
	if err != nil {
		slog.Error("SetIPaddrInUse()", "err", err, "rec", rec)
	}
	fmt.Println("rec=", string(byteJson))

	return d.PutJSON(key, rec)
}

func (d *Database) GetAllocatedIPs(vnetId, ipnetId string) ([]api.IPAddress, error) {
	net, err := d.GetIpNetworkById(vnetId, ipnetId)
	if err != nil {
		slog.Error("GetIpNetworkById()", "err", err, "vnetId", vnetId, "ipnetId", ipnetId)
		return nil, err
	}

	keyPrefix := NetworkPrefix + "/" + vnetId + "/ip_network/" + ipnetId + "/ip_address/" + *net.AddressMaskLen + "/"
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

// PrefixLenToMask はプレフィックス長からネットマスク文字列を生成する（IPv4/IPv6 対応）
// isIPv6 が false なら IPv4 (0〜32)、true なら IPv6 (0〜128)
func PrefixLenToMask(bits int, isIPv6 bool) (string, error) {
	if isIPv6 {
		return ipv6Mask(bits)
	}
	return ipv4Mask(bits)
}

func ipv4Mask(bits int) (string, error) {
	// 範囲チェックを Prefix() に委譲
	if _, err := netip.AddrFrom4([4]byte{}).Prefix(bits); err != nil {
		return "", fmt.Errorf("invalid IPv4 prefix length %d: %w", bits, err)
	}

	mask := uint32(0xFFFFFFFF) << (32 - bits)
	addr := netip.AddrFrom4([4]byte{
		byte(mask >> 24),
		byte(mask >> 16),
		byte(mask >> 8),
		byte(mask),
	})
	return addr.String(), nil
}

func ipv6Mask(bits int) (string, error) {
	// 範囲チェックを Prefix() に委譲
	if _, err := netip.AddrFrom16([16]byte{}).Prefix(bits); err != nil {
		return "", fmt.Errorf("invalid IPv6 prefix length %d: %w", bits, err)
	}

	// 128ビットを上位64bit・下位64bitに分けて計算
	var hi, lo uint64
	switch {
	case bits == 0:
		hi, lo = 0, 0
	case bits <= 64:
		hi = ^uint64(0) << (64 - bits)
		lo = 0
	default:
		hi = ^uint64(0)
		lo = ^uint64(0) << (128 - bits)
	}

	// [16]byte に展開
	var b [16]byte
	for i := range 8 {
		b[i] = byte(hi >> (56 - i*8))
		b[i+8] = byte(lo >> (56 - i*8))
	}

	return netip.AddrFrom16(b).String(), nil
}
