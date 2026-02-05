package db

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	etcd "go.etcd.io/etcd/client/v3"
)

const (
	SERVER_UNKNOWN      = 0 // 不明
	SERVER_PROVISIONING = 1 // プロビジョニング中
	SERVER_RUNNING      = 2 // 実行中
	SERVER_STOPPED      = 3 // 停止中
	SERVER_ERROR        = 4 // エラー状態
	SERVER_DELETING     = 5 // 削除中
	SERVER_DELETED      = 6 // 削除済み
)

var ServerStatus = map[int]string{
	0: "UNKNOWN",
	1: "PROVISIONING",
	2: "RUNNING",
	3: "STOPPED",
	4: "ERROR",
	5: "DELETING",
	6: "DELETED",
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
		server.Uuid = util.StringPtr(uuid.New().String())
		server.Id = (*server.Uuid)[:8]
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

	slog.Debug("GetServers()", "key-prefix", ServerPrefix)
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

	fmt.Println("=== 書き込みデータの情報確認 ===", "server Id", id)
	data3, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		slog.Error("json.MarshalIndent()", "err", err)
	} else {
		fmt.Println("サーバー情報(server): ", string(data3))
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
