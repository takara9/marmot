package db

import (
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	etcd "go.etcd.io/etcd/client/v3"
)

const (
	SERVER_PROVISIONING = 0 // プロビジョニング中
	SERVER_INUSE        = 1 // 使用中
	SERVER_AVAILABLE    = 2 // 利用可能
)

var ServerStatus = map[int]string{
	0: "PROVISIONING",
	1: "INUSE",
	2: "AVAILABLE",
}

// サーバーを登録、サーバーを一意に識別するIDを自動生成
func (d *Database) CreateServer(spec api.Server) (api.Server, error) {
	d.LockKey("/lock/server/create")
	defer d.UnlockKey(d.Mutex)

	// DeepCopyでspecの内容をコピー
	server, err := util.DeepCopy(spec)
	if err != nil {
		slog.Error("CreateServer()", "err", err)
		return api.Server{}, err
	}

	//一意なIDを発行
	var key string
	for {
		server.Id = uuid.New().String()[:8]
		key = ServerPrefix + "/" + server.Id
		_, err := d.GetJSON(key, &server)
		if err == ErrNotFound {
			break
		} else if err != nil {
			slog.Error("CreateServer()", "err", err)
			return api.Server{}, err
		}
	}

	// ステータスセット
	server.Status = util.IntPtrInt(SERVER_PROVISIONING)

	// データベースに登録
	if err := d.PutJSON(key, server); err != nil {
		slog.Error("failed to write database data", "err", err, "key", key)
		return api.Server{}, err
	}

	return server, nil

}

// サーバーをIDで削除
func (d *Database) DeleteServerById(id string) error {
	lockKey := "/lock/server/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockKey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)
	key := ServerPrefix + "/" + id
	return d.DeleteJSON(key)
}

// サーバーをIDで取得
func (d *Database) GetServerById(id string) (api.Server, error) {
	key := ServerPrefix + "/" + id
	var server api.Server
	_, err := d.GetJSON(key, &server)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return api.Server{}, err
	}
	return server, nil
}

// サーバーのリストを取得
func (d *Database) GetServers() (api.Servers, error) {
	var servers []api.Server
	var err error
	var resp *etcd.GetResponse

	resp, err = d.GetByPrefix(ServerPrefix)
	if err == ErrNotFound {
		slog.Debug("no volumes found", "key-prefix", ServerPrefix)
		return servers, nil
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", ServerPrefix)
		return servers, err
	}

	for _, kv := range resp.Kvs {
		var server api.Server
		err := json.Unmarshal([]byte(kv.Value), &server)
		if err != nil {
			slog.Error("Unmarshal() failed", "err", err, "key", string(kv.Key))
			continue
		}
		servers = append(servers, server)
	}

	return servers, nil
}

// サーバーを更新
func (d *Database) UpdateServer(id string, spec api.Server) error {
	for {
		err := d.updateServer(id, spec)
		if err == ErrUpdateConflict {
			slog.Warn("UpdateServer() retrying due to update conflict", "serverId", id)
			continue
		} else if err != nil {
			slog.Error("UpdateServer()", "err", err)
			return err
		}
		break
	}
	return nil
}

// サーバーを更新
func (d *Database) updateServer(id string, spec api.Server) error {
	lockKey := "/lock/server/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockKey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	var rec api.Server
	key := ServerPrefix + "/" + id
	resp, err := d.GetJSON(key, &rec)
	if err != nil {
		slog.Error("GetJSON() failed", "err", err, "key", key)
		return err
	}
	expected := resp.Kvs[0].ModRevision

	rec.Id = spec.Id
	util.Assign(&rec.HvIpAddr, spec.HvIpAddr)
	util.Assign(&rec.HvNode, spec.HvNode)
	util.Assign(&rec.HvPort, spec.HvPort)
	util.Assign(&rec.CTime, spec.CTime)
	util.Assign(&rec.ClusterName, spec.ClusterName)
	util.Assign(&rec.Comment, spec.Comment)
	util.Assign(&rec.Cpu, spec.Cpu)
	util.Assign(&rec.Key, spec.Key)
	util.Assign(&rec.Memory, spec.Memory)
	util.Assign(&rec.Name, spec.Name)
	util.Assign(&rec.OsLv, spec.OsLv)
	util.Assign(&rec.OsVariant, spec.OsVariant)
	util.Assign(&rec.OsVg, spec.OsVg)
	util.Assign(&rec.Playbook, spec.Playbook)
	util.Assign(&rec.PrivateIp, spec.PrivateIp)
	util.Assign(&rec.PublicIp, spec.PublicIp)
	util.Assign(&rec.STime, spec.STime)
	util.Assign(&rec.Status, spec.Status)
	/*
		for _, vol := range *spec.Storage {
			var stg api.Volume
			stg.Id = vol.Id
			util.Assign(&stg.CTime, vol.CTime)
			util.Assign(&stg.Comment, vol.Comment)
			util.Assign(&stg.Key, vol.Key)
			util.Assign(&stg.Kind, vol.Kind)
			util.Assign(&stg.LogicalVolume, vol.LogicalVolume)
			util.Assign(&stg.MTime, vol.MTime)
			util.Assign(&stg.Name, vol.Name)
			util.Assign(&stg.OsName, vol.OsName)
			util.Assign(&stg.OsVersion, vol.OsVersion)
			util.Assign(&stg.Path, vol.Path)
			util.Assign(&stg.Size, vol.Size)
			util.Assign(&stg.Status, vol.Status)
			util.Assign(&stg.Type, vol.Type)
			util.Assign(&stg.VolumeGroup, vol.VolumeGroup)
			*spec.Storage = append(*spec.Storage, stg)
		}
	*/

	err = d.PutJSONCAS(key, expected, &rec)
	if err != nil {
		slog.Error("PutJSONCAS() failed", "err", err, "key", key, "expected", expected)
		return err
	}
	return nil
}
