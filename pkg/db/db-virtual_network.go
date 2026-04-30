package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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
	NETWORK_WAITING      = 6 // ヘッドノードの作成完了待ち

	// Network label keys for distributed sync
	NetworkLabelSyncRole      = "syncRole"
	NetworkLabelHeadNetworkID = "headNetworkId"
	NetworkLabelHeadNodeName  = "headNodeName"
)

var NetworkStatus = map[int]string{
	0: "PENDING",
	1: "PROVISIONING",
	2: "ACTIVE",
	3: "INACTIVE",
	4: "ERROR",
	5: "DELETING",
	6: "WAITING",
}

// SetNetworkSyncLabels sets distributed sync metadata labels for networks.
func SetNetworkSyncLabels(labels map[string]interface{}, syncRole, headNetworkID, headNodeName string) {
	if labels == nil {
		return
	}
	labels[NetworkLabelSyncRole] = strings.TrimSpace(syncRole)
	labels[NetworkLabelHeadNetworkID] = strings.TrimSpace(headNetworkID)
	if node := strings.TrimSpace(headNodeName); node != "" {
		labels[NetworkLabelHeadNodeName] = node
	}
}

func GetNetworkSyncRole(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[NetworkLabelSyncRole].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func GetHeadNetworkID(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[NetworkLabelHeadNetworkID].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func GetHeadNetworkNodeName(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[NetworkLabelHeadNodeName].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func (d *Database) getUniqueVirtualNetworkID() (string, error) {
	for {
		id := uuid.New().String()[:5]
		key := NetworkPrefix + "/" + id
		var network api.VirtualNetwork
		_, err := d.GetJSON(key, &network)
		if err == ErrNotFound {
			return id, nil
		}
		if err != nil {
			slog.Error("getUniqueVirtualNetworkID()", "err", err)
			return "", err
		}
	}
}

// 仮想ネットワークを登録、仮想ネットワークを一意に識別するIDを自動生成
func (d *Database) CreateVirtualNetwork(spec api.VirtualNetwork) (api.VirtualNetwork, error) {
	d.LockKey("/lock/virtualnetwork/create")
	defer d.UnlockKey(d.Mutex)

	// 同一ネットワーク名のネットワークが存在しないか確認
	networks, err := d.GetVirtualNetworks()
	if err != nil {
		slog.Error("CreateVirtualNetwork() GetVirtualNetworks() failed", "err", err)
		return api.VirtualNetwork{}, err
	}
	for _, n := range networks {
		if n.Metadata.Name != nil && spec.Metadata.Name != nil && *n.Metadata.Name == *spec.Metadata.Name {
			err := fmt.Errorf("network with the same name already exists: %s", *spec.Metadata.Name)
			slog.Error("CreateVirtualNetwork()", "err", err)
			return api.VirtualNetwork{}, err
		}
	}

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

	// ブリッジ名の自動生成
	if network.Spec.BridgeName == nil {
		bridgeName := "br-" + network.Id
		network.Spec.BridgeName = util.StringPtr(bridgeName)
	}

	// ステータスセット、タイムスタンプセット
	var s api.Status
	s.StatusCode = NETWORK_PENDING
	s.Status = util.StringPtr(NetworkStatus[s.StatusCode])
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
		if strings.Contains(string(kv.Key), "/ip_network/") {
			continue
		}
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
		slog.Error("GetVirtualNetworkById()", "err", err, "id", id, "key", key) // ここでエラーが発生する。なぜ？？ not found
		return api.VirtualNetwork{}, err
	}
	return network, nil
}

// 仮想ネットワークのリストを取得
func (d *Database) GetVirtualNetworkByName(name string) (api.VirtualNetwork, error) {
	var err error
	var resp *etcd.GetResponse

	slog.Debug("仮想ネットワークを名前で検索 GetVirtualNetworkByName()", "key-prefix", NetworkPrefix, "name", name)
	resp, err = d.GetByPrefix(NetworkPrefix)
	if err == ErrNotFound {
		slog.Debug("no networks found", "key-prefix", NetworkPrefix)
		return api.VirtualNetwork{}, ErrNotFound
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", NetworkPrefix)
		return api.VirtualNetwork{}, err
	}

	for _, kv := range resp.Kvs {
		//fmt.Println("仮想ネットワークの検索結果", "key", string(kv.Key), "value", string(kv.Value))
		if strings.Contains(string(kv.Key), "/ip_network/") {
			continue
		}
		var network api.VirtualNetwork
		err := json.Unmarshal([]byte(kv.Value), &network)
		if err != nil {
			slog.Error("Unmarshal() failed", "err", err, "key", string(kv.Key))
			continue
		}
		if network.Metadata.Name != nil && *network.Metadata.Name == name {
			slog.Debug("仮想ネットワークを名前で検索結果", "key", string(kv.Key), "name", name)
			return network, nil
		}
	}

	slog.Debug("仮想ネットワークを名前で検索結果なし", "name", name)
	return api.VirtualNetwork{}, ErrNotFound
}

// 仮想ネットワークをIDで削除
func (d *Database) DeleteVirtualNetworkById(vnetId string) error {
	// ネットワークの存在確認
	network, err := d.GetVirtualNetworkById(vnetId)
	if err != nil {
		slog.Error("DeleteVirtualNetworkById() GetVirtualNetworkById() failed", "err", err, "networkId", vnetId) // ここでエラーが発生する。なぜ？？ not found
		return err
	}

	// IPネットワークの削除
	if network.Spec.IpNetworkId != nil {
		err := d.DeleteIpNetworkById(vnetId, *network.Spec.IpNetworkId)
		if err == ErrNotFound {
			slog.Error("Not Found ", "err", err, "ipNetworkId", *network.Spec.IpNetworkId)
		} else if err != nil {
			slog.Error("DeleteVirtualNetworkById() DeleteIpNetworkById() failed", "err", err, "ipNetworkId", *network.Spec.IpNetworkId)
			return err
		}
	}

	// ネットワークの削除
	lockKey := "/lock/virtualnetwork/" + vnetId
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockKey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)
	key := NetworkPrefix + "/" + vnetId
	return d.DeleteJSON(key)
}

// 仮想ネットワークを更新
func (d *Database) UpdateVirtualNetworkById(vnetId string, spec api.VirtualNetwork) error {
	for {
		err := d.updateVirtualNetwork(vnetId, spec)
		if err == ErrUpdateConflict {
			slog.Warn("UpdateVirtualNetwork() retrying due to update conflict", "networkId", vnetId)
			continue
		} else if err != nil {
			slog.Error("UpdateVirtualNetwork()", "err", err)
			return err
		}
		break
	}

	fmt.Println("=== 書き込みデータの情報確認 ===", "network Id", vnetId)
	data3, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		slog.Error("json.MarshalIndent()", "err", err)
	} else {
		fmt.Println("仮想ネットワーク情報(network): ", string(data3))
	}

	return nil
}

// 内部関数 仮想ネットワークを更新
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
	network.Status.StatusCode = status
	network.Status.Status = util.StringPtr(NetworkStatus[network.Status.StatusCode])
	network.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	if status == NETWORK_ACTIVE {
		network.Status.Message = nil
	}
	if err := d.UpdateVirtualNetworkById(id, network); err != nil {
		slog.Error("UpdateVirtualNetworkStatus() UpdateVirtualNetwork() failed", "err", err, "networkId", id)
		panic(err)
	}
}

// 仮想ネットワークオブジェクトの削除タイムスタンプをセット
func (d *Database) SetDeleteTimestampVirtualNetwork(id string) error {
	network, err := d.GetVirtualNetworkById(id)
	if err != nil {
		slog.Error("SetDeleteTimestamp() GetVirtualNetworkById() failed", "err", err, "networkId", id)
		return err
	}
	network.Status.DeletionTimeStamp = util.TimePtr(time.Now())
	if err := d.UpdateVirtualNetworkById(id, network); err != nil {
		slog.Error("SetDeleteTimestamp() UpdateVirtualNetworkById() failed", "err", err, "networkId", id)
		return err
	}
	return nil
}

// 既存ネットワークの取得とデータベースへの登録
func (d *Database) PutVirtualNetworksETCD(vnet api.VirtualNetwork) error {
	slog.Debug("PutVirtualNetworksETCD called")

	// 作成したオブジェクトをデータベースに保存
	d.LockKey("/lock/virtualnetwork/create")
	defer d.UnlockKey(d.Mutex)

	// DeepCopyでvnetの内容をコピー
	network, err := util.DeepCopy(vnet)
	if err != nil {
		slog.Error("CreateVirtualNetwork()", "err", err)
		return err
	}

	// IDは一意であると信じて処理
	key := NetworkPrefix + "/" + vnet.Id

	// ステータスセット、タイムスタンプセット
	var s api.Status
	s.StatusCode = NETWORK_PENDING
	s.Status = util.StringPtr(NetworkStatus[s.StatusCode])
	s.CreationTimeStamp = util.TimePtr(time.Now())
	s.LastUpdateTimeStamp = util.TimePtr(time.Now())
	network.Status = &s
	if err := d.PutJSON(key, network); err != nil {
		slog.Error("failed to write database data", "err", err, "key", key)
		return err
	}

	return nil
}

// MakeFollowerVirtualNetworkEntry creates a follower-side network object
// that waits for the head network to become ACTIVE.
func (d *Database) MakeFollowerVirtualNetworkEntry(headNetwork api.VirtualNetwork, followerNodeName string, headNetworkID string) (string, error) {
	followerNodeName = strings.TrimSpace(followerNodeName)
	headNetworkID = strings.TrimSpace(headNetworkID)
	if followerNodeName == "" {
		return "", fmt.Errorf("follower nodeName is required")
	}
	if headNetworkID == "" {
		return "", fmt.Errorf("head network id is required")
	}
	if headNetwork.Metadata == nil || headNetwork.Metadata.Name == nil || strings.TrimSpace(*headNetwork.Metadata.Name) == "" {
		return "", fmt.Errorf("head network metadata.name is required")
	}

	headNodeName := ""
	if headNetwork.Metadata.NodeName != nil {
		headNodeName = strings.TrimSpace(*headNetwork.Metadata.NodeName)
	}
	if headNodeName == "" {
		return "", fmt.Errorf("head network nodeName is required")
	}

	networks, err := d.GetVirtualNetworks()
	if err != nil {
		return "", err
	}
	for _, n := range networks {
		if n.Metadata == nil || n.Metadata.NodeName == nil || n.Metadata.Labels == nil {
			continue
		}
		if strings.TrimSpace(*n.Metadata.NodeName) != followerNodeName {
			continue
		}
		labels := *n.Metadata.Labels
		if GetNetworkSyncRole(labels) == "follower" && GetHeadNetworkID(labels) == headNetworkID {
			return n.Id, nil
		}
	}

	id, err := d.getUniqueVirtualNetworkID()
	if err != nil {
		return "", err
	}

	labels := map[string]interface{}{}
	SetNetworkSyncLabels(labels, "follower", headNetworkID, headNodeName)

	var spec *api.VirtualNetworkSpec
	if headNetwork.Spec != nil {
		specCopy, copyErr := util.DeepCopy(*headNetwork.Spec)
		if copyErr != nil {
			return "", copyErr
		}
		spec = &specCopy
	}

	follower := api.VirtualNetwork{
		Id: id,
		Metadata: &api.Metadata{
			Name:     headNetwork.Metadata.Name,
			NodeName: util.StringPtr(followerNodeName),
			Labels:   &labels,
		},
		Spec: spec,
		Status: &api.Status{
			StatusCode:          NETWORK_WAITING,
			Status:              util.StringPtr(NetworkStatus[NETWORK_WAITING]),
			CreationTimeStamp:   util.TimePtr(time.Now()),
			LastUpdateTimeStamp: util.TimePtr(time.Now()),
			Message:             util.StringPtr("ヘッドノードのネットワーク作成完了を待機中"),
		},
	}

	if follower.Spec != nil {
		follower.Spec.IpNetworkId = nil
	}

	key := NetworkPrefix + "/" + id
	if err := d.PutJSON(key, follower); err != nil {
		slog.Error("MakeFollowerVirtualNetworkEntry() PutJSON failed", "err", err)
		return "", err
	}

	return id, nil
}
