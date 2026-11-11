package db

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/takara9/marmot/api"
	etcd "go.etcd.io/etcd/client/v3"
)

const (
	HostStatusPrefix = "/marmot/hoststatus"
)

func resolveHostStatusCreationTimestamp(incoming api.HostStatus, existing *api.HostStatus, now time.Time) *time.Time {
	if incoming.CreationTimeStamp != nil {
		return incoming.CreationTimeStamp
	}
	if existing != nil {
		if existing.CreationTimeStamp != nil {
			return existing.CreationTimeStamp
		}
		if existing.LastUpdated != nil {
			return existing.LastUpdated
		}
	}
	return &now
}

const hostStatusCASRetries = 10

// HostStatusを現在のリビジョンと共に取得する。存在しない場合は existing=nil、rev=0
func (d *Database) getHostStatusWithRevision(key string) (*api.HostStatus, int64, error) {
	var stored api.HostStatus
	resp, err := d.GetJSON(key, &stored)
	if err == ErrNotFound {
		return nil, 0, nil
	} else if err != nil {
		return nil, 0, err
	}
	return &stored, resp.Kvs[0].ModRevision, nil
}

// HostStatusをetcdに保存する
func (d *Database) PutHostStatus(status api.HostStatus) error {
	if status.NodeName == nil {
		return nil
	}
	key := HostStatusPrefix + "/" + *status.NodeName
	incoming := status
	for i := 0; i < hostStatusCASRetries; i++ {
		existing, rev, err := d.getHostStatusWithRevision(key)
		if err != nil {
			return err
		}
		status = incoming
		status.CreationTimeStamp = resolveHostStatusCreationTimestamp(status, existing, time.Now())
		// 定期更新が、クラスタ離脱中の配置禁止を上書きしないよう引き継ぐ
		if status.Unschedulable == nil && existing != nil {
			status.Unschedulable = existing.Unschedulable
		}
		err = d.PutJSONCAS(key, rev, status)
		if err != ErrUpdateConflict {
			return err
		}
	}
	return ErrUpdateConflict
}

// ノードを新規サーバーの配置対象から外す（または戻す）
func (d *Database) SetHostUnschedulable(nodeName string, unschedulable bool) error {
	key := HostStatusPrefix + "/" + nodeName
	for i := 0; i < hostStatusCASRetries; i++ {
		status, rev, err := d.getHostStatusWithRevision(key)
		if err != nil {
			return err
		}
		if status == nil {
			return ErrNotFound
		}
		status.Unschedulable = &unschedulable
		err = d.PutJSONCAS(key, rev, *status)
		if err != ErrUpdateConflict {
			return err
		}
	}
	return ErrUpdateConflict
}

// HostStatusをetcdから取得する
func (d *Database) GetHostStatus(nodeName string) (api.HostStatus, error) {
	key := HostStatusPrefix + "/" + nodeName
	var status api.HostStatus
	_, err := d.GetJSON(key, &status)
	if err != nil {
		return api.HostStatus{}, err
	}
	return status, nil
}

// 全HostStatusをetcdから取得する
func (d *Database) GetAllHostStatus() ([]api.HostStatus, error) {
	var statuses []api.HostStatus
	var resp *etcd.GetResponse

	resp, err := d.GetByPrefix(HostStatusPrefix)
	if err == ErrNotFound {
		slog.Debug("no host status found", "key-prefix", HostStatusPrefix)
		return statuses, nil
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", HostStatusPrefix)
		return statuses, err
	}

	for _, kv := range resp.Kvs {
		var status api.HostStatus
		if err := json.Unmarshal(kv.Value, &status); err != nil {
			slog.Error("Unmarshal() failed", "err", err, "key", string(kv.Key))
			continue
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}
