package marmotd

// イメージの情報管理の関数群

import (
	"errors"
	"log/slog"

	"github.com/takara9/marmot/api"
)


// CreateNewImage は、指定されたIDのイメージを新規作成する関数
// コントローラーから呼び出されることを想定している
func (m *Marmot) CreateNewImage(id string) (*api.Image, error) {
	slog.Debug("Creating image", "imgId", id)

	// URLからイメージをダウンロードして保存する処理などをここに実装する予定
	// 例えば、イメージのURLがAPIから取得できると仮定して、そのURLからイメージをダウンロードして保存する処理など

	// 時間を要する処理は、ジョブに依頼して、非同期に実行することも検討する必要があるかもしれない

	// ここでは、まだ実装していないため、エラーを返す

	return &api.Image{}, errors.New("not implemented")
}


func (m *Marmot) GetImages() (*api.Image, error) {
	slog.Debug("Getting images")

	// DBからイメージの情報を取得する処理などをここに実装する予定
	// 例えば、DBからイメージの情報を取得して、それをAPIのレスポンスとして返す処理など

	// ここでは、まだ実装していないため、エラーを返す

	return &api.Image{}, errors.New("not implemented")
}

func (m *Marmot) GetImage(id string) (*api.Image, error) {
	slog.Debug("Getting image", "imgId", id)

	// DBから指定されたIDのイメージの情報を取得する処理などをここに実装する予定
	// 例えば、DBから指定されたIDのイメージの情報を取得して、それをAPIのレスポンスとして返す処理など

	// ここでは、まだ実装していないため、エラーを返す

	return &api.Image{}, errors.New("not implemented")
}	

func (m *Marmot) DeleteImage(id string) error {
	slog.Debug("Deleting image", "imgId", id)

	// DBから指定されたIDのイメージの情報を削除する処理などをここに実装する予定
	// 例えば、DBから指定されたIDのイメージの情報を削除して、それをAPIのレスポンスとして返す処理など

	// ここでは、まだ実装していないため、エラーを返す

	return errors.New("not implemented")
}	

func (m *Marmot) UpdateImage(id string, image *api.Image) error {
	slog.Debug("Updating image", "imgId", id)

	// DBから指定されたIDのイメージの情報を更新する処理などをここに実装する予定
	// 例えば、DBから指定されたIDのイメージの情報を更新して、それをAPIのレスポンスとして返す処理など

	// ここでは、まだ実装していないため、エラーを返す

	return errors.New("not implemented")
}