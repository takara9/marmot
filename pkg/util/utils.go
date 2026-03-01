package util

import (
	"crypto/rand"
	"encoding/json"
	"net"
	"os"
	"reflect"
	"time"

	"github.com/jinzhu/copier"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
)

// ポインタ生成ユーティリティ関数群
func BoolPtr(b bool) *bool           { return &b }
func IntPtrInt32(i int) *int32       { j := int32(i); return &j }
func IntPtrInt(i int) *int           { j := i; return &j }
func IntPtrInt64(i int) *int64       { j := int64(i); return &j }
func Int64PtrInt32(i uint64) *int32  { j := int32(i); return &j }
func Int64PtrConvMB(i uint64) *int64 { j := int64(i) * 1024; return &j }
func StringPtr(s string) *string     { return &s }
func TimePtr(t time.Time) *time.Time { return &t }

// ポインタがNULLの時、デフォルト値取得ユーティリティ関数
func OrDefault[T any](p *T, d T) T {
	if p != nil {
		return *p
	}
	return d
}

// ポインタがNULLでなければ代入するユーティリティ関数
func Assign[T any](dst **T, src *T) {
	if src != nil {
		*dst = src
	}
}

// DeepCopy performs a deep copy of a generic type T using JSON serialization.
func DeepCopy[T any](src T) (T, error) {
	b, err := json.Marshal(src)
	if err != nil {
		return src, err
	}
	var dst T
	err = json.Unmarshal(b, &dst)
	return dst, err
}

func ConvertToJSON(src *config.VirtualNetwork) (*api.VirtualNetwork, error) {
	dst := &api.VirtualNetwork{}
	if err := copier.CopyWithOption(dst, src, copier.Option{
		DeepCopy: true,
	}); err != nil {
		return nil, err
	}
	return dst, nil
}

// src の非ゼロ値フィールドだけを dst にコピーする
// dst と src は同じ型であることを想定
func PatchStruct(dst, src interface{}) {
	dv := reflect.ValueOf(dst)
	sv := reflect.ValueOf(src)

	// ポインタの中身を取得
	if dv.Kind() == reflect.Ptr {
		dv = dv.Elem()
	}
	if sv.Kind() == reflect.Ptr {
		sv = sv.Elem()
	}

	// 構造体でない場合は何もしない
	if dv.Kind() != reflect.Struct || sv.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < sv.NumField(); i++ {
		sf := sv.Field(i)
		df := dv.FieldByName(sv.Type().Field(i).Name) // 名前でフィールドを特定

		// フィールドが存在しない、または書き込み不可ならスキップ
		if !df.IsValid() || !df.CanSet() {
			continue
		}

		// srcのフィールドがゼロ値ならスキップ（オプション）
		if sf.IsZero() {
			continue
		}

		// 再帰処理の判定
		if sf.Kind() == reflect.Struct {
			// 構造体同士なら再帰的にパッチ
			PatchStruct(df.Addr().Interface(), sf.Interface())
		} else {
			// 基本型ならそのままコピー
			df.Set(sf)
		}
	}
}

// GenerateRandomMAC はランダムなMACアドレスを生成します
func GenerateRandomMAC() (net.HardwareAddr, error) {
	buf := make([]byte, 6)
	_, err := rand.Read(buf)
	if err != nil {
		return nil, err
	}

	// 最初のバイトの特定のビットを操作して「ローカル管理アドレス」に設定します
	// 0x02 ビットを立てることで、ユニバーサル管理ではなくローカル管理であることを示します
	// 0x01 ビットを落とすことで、マルチキャストではなくユニキャストであることを示します
	buf[0] = (buf[0] | 2) & 0xfe

	return net.HardwareAddr(buf), nil
}

const channelBasePath = "/run/libvirt/qemu/channel/"

// ホストのチャネル用ディレクトリを作成
func CreateChannelDir(hostname string) (string, error) {
	dirPath := channelBasePath + "/" + hostname
	err := os.MkdirAll(dirPath, 0755)
	if err != nil {
		return "", err
	}
	return dirPath, nil
}

// ホストのチャネル用ディレクトリを削除
func RemoveChannelDir(hostname string) error {
	dirPath := channelBasePath + "/" + hostname
	return os.RemoveAll(dirPath)
}
