package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	etcd "go.etcd.io/etcd/client/v3"
)

const (
	NETWORK_PENDING      = 0 // 待ち状態
	NETWORK_PROVISIONING = 1 // プロビジョニング中
	NETWORK_ACTIVE       = 2 // 活性中
	NETWORK_INACTIVE     = 3 // 不活性中
	NETWORK_ERROR        = 4 // エラー状態
	NETWORK_DELETING     = 5 // 削除中
)

var NetworkStatus = map[int]string{
	0: "PENDING",
	1: "PROVISIONING",
	2: "ACTIVE",
	3: "INACTIVE",
	4: "ERROR",
	5: "DELETING",
}

// 仮想ネットワークを登録、仮想ネットワークを一意に識別するIDを自動生成
func (d *Database) CreateVirtualNetwork(spec api.VirtualNetwork) (api.VirtualNetwork, error) {
	d.LockKey("/lock/virtualnetwork/create")
	defer d.UnlockKey(d.Mutex)

	// DeepCopyでspecの内容をコピー
	network, err := util.DeepCopy(spec)
	if err != nil {
		slog.Error("CreateVirtualNetwork()", "err", err)
		return api.VirtualNetwork{}, err
	}

	//一意なIDを発行
	var key string
	for {
		network.Metadata.Uuid = util.StringPtr(uuid.New().String())
		network.Id = (*network.Metadata.Uuid)[:5]
		key = NetworkPrefix + "/" + network.Id
		_, err := d.GetJSON(key, &network)
		if err == ErrNotFound {
			break
		} else if err != nil {
			slog.Error("CreateVirtualNetwork()", "err", err)
			return api.VirtualNetwork{}, err
		}
	}

	// ステータスセット、タイムスタンプセット
	var s api.Status
	s.Status = util.IntPtrInt(NETWORK_PENDING)
	s.CreationTimeStamp = util.TimePtr(time.Now())
	s.LastUpdateTimeStamp = util.TimePtr(time.Now())
	network.Status = &s
	if err := d.PutJSON(key, network); err != nil {
		slog.Error("failed to write database data", "err", err, "key", key)
		return api.VirtualNetwork{}, err
	}

	return network, nil

}

// 仮想ネットワークのリストを取得
func (d *Database) GetVirtualNetworks() ([]api.VirtualNetwork, error) {
	var networks []api.VirtualNetwork
	var err error
	var resp *etcd.GetResponse

	slog.Debug("GetVirtualNetworks()", "key-prefix", NetworkPrefix)
	resp, err = d.GetByPrefix(NetworkPrefix)
	if err == ErrNotFound {
		slog.Debug("no networks found", "key-prefix", NetworkPrefix)
		return networks, nil
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", NetworkPrefix)
		return networks, err
	}

	for _, kv := range resp.Kvs {
		var network api.VirtualNetwork
		err := json.Unmarshal([]byte(kv.Value), &network)
		if err != nil {
			slog.Error("Unmarshal() failed", "err", err, "key", string(kv.Key))
			continue
		}
		networks = append(networks, network)
	}

	return networks, nil
}

// 仮想ネットワークをIDで取得
func (d *Database) GetVirtualNetworkById(id string) (api.VirtualNetwork, error) {
	key := NetworkPrefix + "/" + id
	var network api.VirtualNetwork
	_, err := d.GetJSON(key, &network)
	if err != nil {
		slog.Error("GetVirtualNetworkById()", "err", err)
		return api.VirtualNetwork{}, err
	}
	return network, nil
}

// 仮想ネットワークをIDで削除
func (d *Database) DeleteVirtualNetworkById(id string) error {
	lockKey := "/lock/virtualnetwork/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockKey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)
	key := NetworkPrefix + "/" + id
	return d.DeleteJSON(key)
}

// 仮想ネットワークを更新
func (d *Database) UpdateVirtualNetworkById(id string, spec api.VirtualNetwork) error {
	for {
		err := d.updateVirtualNetwork(id, spec)
		if err == ErrUpdateConflict {
			slog.Warn("UpdateVirtualNetwork() retrying due to update conflict", "networkId", id)
			continue
		} else if err != nil {
			slog.Error("UpdateVirtualNetwork()", "err", err)
			return err
		}
		break
	}

	fmt.Println("=== 書き込みデータの情報確認 ===", "network Id", id)
	data3, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		slog.Error("json.MarshalIndent()", "err", err)
	} else {
		fmt.Println("仮想ネットワーク情報(network): ", string(data3))
	}

	return nil
}

// 仮想ネットワークを更新
func (d *Database) updateVirtualNetwork(id string, spec api.VirtualNetwork) error {
	lockKey := "/lock/virtualnetwork/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockKey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	var rec api.VirtualNetwork
	key := NetworkPrefix + "/" + id
	resp, err := d.GetJSON(key, &rec)
	if err != nil {
		slog.Error("GetJSON() failed", "err", err, "key", key)
		return err
	}
	expected := resp.Kvs[0].ModRevision

	rec.Id = id
	// パッチ適用
	util.PatchStruct(&rec, spec)

	err = d.PutJSONCAS(key, expected, &rec)
	if err != nil {
		slog.Error("PutJSONCAS() failed", "err", err, "key", key, "expected", expected)
		return err
	}
	return nil
}

// 仮想ネットワークオブジェクトのステータスを更新
func (d *Database) UpdateVirtualNetworkStatus(id string, status int) {
	network, err := d.GetVirtualNetworkById(id)
	if err != nil {
		slog.Error("UpdateVirtualNetworkStatus() GetVirtualNetworkById() failed", "err", err, "networkId", id)
		panic(err)
	}
	network.Status.Status = util.IntPtrInt(status)
	network.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	if err := d.UpdateVirtualNetworkById(id, network); err != nil {
		slog.Error("UpdateVirtualNetworkStatus() UpdateVirtualNetwork() failed", "err", err, "networkId", id)
		panic(err)
	}
}

func (d *Database) SetDeleteTimestampVirtualNetwork(id string) error {
	server, err := d.GetServerById(id)
	if err != nil {
		slog.Error("SetDeleteTimestamp() GetServerById() failed", "err", err, "serverId", id)
		return err
	}
	server.Status.DeletionTimeStamp = util.TimePtr(time.Now())
	if err := d.UpdateServer(id, server); err != nil {
		slog.Error("SetDeleteTimestamp() UpdateServer() failed", "err", err, "serverId", id)
		return err
	}
	return nil
}