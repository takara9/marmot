package util

import (
	"strings"

	"github.com/takara9/marmot/api"
)

// NormalizeVolumeSpecISCSIAlias は iSCSI エイリアス入力を正規化する共通ヘルパーです。
// spec.Type が "iscsi" の場合は "lvm" に変換して spec.Iscsi を true に設定します。
// spec.Iscsi が true の場合、Type が未指定なら "lvm" を、Kind が未指定なら "data" を補完します。
func NormalizeVolumeSpecISCSIAlias(spec *api.VolSpec) {
	if spec == nil {
		return
	}
	if spec.Type != nil && strings.EqualFold(strings.TrimSpace(*spec.Type), "iscsi") {
		spec.Type = StringPtr("lvm")
		spec.Iscsi = BoolPtr(true)
	}
	if spec.Iscsi != nil && *spec.Iscsi {
		if spec.Type == nil || strings.TrimSpace(*spec.Type) == "" {
			spec.Type = StringPtr("lvm")
		}
		if spec.Kind == nil || strings.TrimSpace(*spec.Kind) == "" {
			spec.Kind = StringPtr("data")
		}
	}
}
