package util

import (
	"strings"

	"github.com/takara9/marmot/api"
)

// NormalizeVolumeSpecISCSIAlias は iSCSI エイリアス入力を正規化する共通ヘルパーです。
// spec.Type が "iscsi" の場合は "lvm" に変換して spec.Iscsi を true に設定します。
// spec.Iscsi が true の場合、Type/Kind を下流の厳密比較に合わせて正規化し、Kind が data のとき Size が未指定/0以下なら 1GB を補完します。
func NormalizeVolumeSpecISCSIAlias(spec *api.VolSpec) {
	if spec == nil {
		return
	}
	if spec.Type != nil && strings.EqualFold(strings.TrimSpace(*spec.Type), "iscsi") {
		spec.Type = StringPtr("lvm")
		spec.Iscsi = BoolPtr(true)
	}
	if spec.Iscsi != nil && *spec.Iscsi {
		// Canonicalize values used by downstream exact-match checks.
		if spec.Type == nil || strings.TrimSpace(*spec.Type) == "" || strings.EqualFold(strings.TrimSpace(*spec.Type), "lvm") {
			spec.Type = StringPtr("lvm")
		}
		// When iSCSI is enabled, default Kind to "data" and ensure Size has a sane default to avoid nil dereferences downstream.
		if spec.Kind == nil || strings.TrimSpace(*spec.Kind) == "" || strings.EqualFold(strings.TrimSpace(*spec.Kind), "data") {
			spec.Kind = StringPtr("data")
			if spec.Size == nil || *spec.Size <= 0 {
				spec.Size = IntPtrInt(1) // 1GB
			}
		}
	}
}
