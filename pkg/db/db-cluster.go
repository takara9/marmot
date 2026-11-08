package db

import (
	"github.com/takara9/marmot/api"
)

// クラスタ情報（CLUSTER-ID とマスターノード）の保存先。存在すればクラスタ有効
const ClusterInfoKey = "/marmot/cluster"

// ClusterInfoをetcdに保存する
func (d *Database) PutClusterInfo(info api.ClusterInfo) error {
	return d.PutJSON(ClusterInfoKey, info)
}

// ClusterInfoをetcdから取得する。クラスタが無効なら ErrNotFound を返す
func (d *Database) GetClusterInfo() (*api.ClusterInfo, error) {
	var info api.ClusterInfo
	if _, err := d.GetJSON(ClusterInfoKey, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
