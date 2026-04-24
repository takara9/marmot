package db

import (
	"encoding/json"
	"log/slog"

	"github.com/takara9/marmot/api"
	etcd "go.etcd.io/etcd/client/v3"
)

const (
	HostStatusPrefix = "/marmot/hoststatus"
)

// HostStatusをetcdに保存する
func (d *Database) PutHostStatus(status api.HostStatus) error {
	if status.NodeName == nil {
		return nil
	}
	key := HostStatusPrefix + "/" + *status.NodeName
	return d.PutJSON(key, status)
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
