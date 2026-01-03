package util

import (
	"encoding/json"
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
