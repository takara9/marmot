package util

import (
	"crypto/rand"
	"encoding/json"
	"net"
	"os"
	"reflect"
	"time"
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

// src の非ゼロ値フィールドだけを dst にコピーする
// dst と src は同じ型であることを想定
func PatchStruct(dst, src interface{}) {
	// reflect を使ってフィールドごとにコピー
	dv := reflect.ValueOf(dst).Elem()
	sv := reflect.ValueOf(src)

	// コピーループ
	for i := 0; i < dv.NumField(); i++ {
		df := dv.Field(i)
		sf := sv.Field(i)

		// src がゼロ値ならスキップ
		if reflect.Value.IsZero(sf) {
			continue
		}

		// 書き込み可能ならコピー
		if df.CanSet() {
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
