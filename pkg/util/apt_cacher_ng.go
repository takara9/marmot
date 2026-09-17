package util

import "sync/atomic"

var aptCacherNGEnabled atomic.Bool

// SetAptCacherNGEnabled は、新規作成するゲストにAPTプロキシ設定を書き込むかどうかを
// 切り替える(issue #696)。apt-cacher-ngが実際に到達可能な環境でのみ有効化すべきため、
// marmotd側の設定(apt_cacher_ng_enabled)に応じてmarmotd起動時に呼び出される。
func SetAptCacherNGEnabled(enabled bool) {
	aptCacherNGEnabled.Store(enabled)
}

// IsAptCacherNGEnabled は現在の有効/無効状態を返す。
func IsAptCacherNGEnabled() bool {
	return aptCacherNGEnabled.Load()
}
