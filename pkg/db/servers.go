package db

import (
	"fmt"

	"github.com/takara9/marmot/api"
)

// サーバーを登録、サーバーを一意に識別するIDを自動生成
func (d *Database) CreateServer(spec api.Server) (api.Server, error) {
	// サーバー作成ロジックを実装
	return api.Server{}, fmt.Errorf("not implemented")
}

// サーバーをIDで削除
func (d *Database) DeleteServerById(id string) error {
	// サーバー削除ロジックを実装
	return fmt.Errorf("not implemented")
}

// サーバーをIDで取得
func (d *Database) GetServerById(id string) (api.Server, error) {
	// サーバー取得ロジックを実装
	return api.Server{}, fmt.Errorf("not implemented")
}

// サーバーのリストを取得
func (d *Database) GetServers() (api.Servers, error) {
	// サーバー一覧取得ロジックを実装
	return nil, fmt.Errorf("not implemented")
}

// サーバーを更新
func (d *Database) UpdateServer(spec api.Server) error {
	// サーバー更新ロジックを実装
	return fmt.Errorf("not implemented")
}
