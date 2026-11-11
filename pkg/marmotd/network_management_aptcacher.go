package marmotd

import (
	"fmt"
	"os/exec"
	"strings"
)

const aptCacherNGUnit = "apt-cacher-ng.service"

// aptCacherNGSystemctl はテストから差し替え可能にするための systemctl 呼び出し。
var aptCacherNGSystemctl = func(args ...string) error {
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// EnsureManagementAptCacher は、10.245.0.1 を持つ mgmt の head ノードだけで apt-cacher-ng を稼働させる(issue #696)。
// head なら起動・有効化し、follower なら停止・無効化する。apt_cacher_ng_enabled が false の場合は何もしない。
// 状態を見てから変更するため、定期ループから繰り返し呼んでも副作用は残らない。
func EnsureManagementAptCacher(cfg *MarmotdConfig, head bool) error {
	if cfg == nil || !cfg.AptCacherNGEnabled {
		return nil
	}

	active := aptCacherNGSystemctl("is-active", "--quiet", aptCacherNGUnit) == nil
	enabled := aptCacherNGSystemctl("is-enabled", "--quiet", aptCacherNGUnit) == nil

	if head {
		if active && enabled {
			return nil
		}
		return aptCacherNGSystemctl("enable", "--now", aptCacherNGUnit)
	}
	if !active && !enabled {
		return nil
	}
	return aptCacherNGSystemctl("disable", "--now", aptCacherNGUnit)
}
