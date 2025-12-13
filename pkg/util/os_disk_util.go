package util

/*
  LVM関連のユーテリティ関数群

 　* LVのパーティションをマップ
   * LVのパーティションをアンマップ
   * LVをマウントポイントへマウント
   * LVをアンマウント
   * OSテンプVolのスナップショットを作成してデバイス名を返す
   * データボリュームの作成
   * SSボリュームをマウントして、ホスト名とIPアドレスを設定

*/

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/takara9/marmot/api"
)

// LVのパーティションをマップ
func KpartOn(vg string, lv string) error {
	cmd := exec.Command("kpartx", "-av", fmt.Sprintf("/dev/%s/%s", vg, lv))
	err := cmd.Run()
	return err
}

// LVのパーティションをアンマップ
func KpartOff(vg string, lv string) error {
	cmd := exec.Command("kpartx", "-d", fmt.Sprintf("/dev/%s/%s", vg, lv))
	err := cmd.Run()
	return err
}

// LVをマウントポイントへマウント
func MountLocal(vg string, lv string, uuid string) (string, error) {
	dev := fmt.Sprintf("/dev/mapper/%s-%sp2", vg, lv)
	mp := fmt.Sprintf("./%s", uuid)
	err := os.Mkdir(mp, 0750)
	if err != nil && !os.IsExist(err) {
		err := errors.New("failed mkdir to setup OS-Disk")
		return "", err
	}
	cmd := exec.Command("mount", "-t", "ext4", dev, mp)
	err = cmd.Run()
	if err != nil {
		err := errors.New("mount failed to setup OS-Disk")
		return "", err
	}
	return mp, nil
}

// LVをアンマウント
func UnMountLocal(uuid string) error {
	mp := fmt.Sprintf("./%s", uuid)
	cmd := exec.Command("umount", mp)
	err := cmd.Run()
	os.RemoveAll(mp)
	return err
}

// スナップショットボリュームをマウントして、 ホスト名とIPアドレスを設定
//
//	Ubuntu Linuxに限定
func ConfigRootVol(spec api.VmSpec, vg string, oslv string) error {
	err := KpartOn(vg, oslv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	// マウント
	vm_root, err := MountLocal(vg, oslv, *spec.Uuid)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// ホスト名の書き込み
	err = LinuxSetup_hostname(vm_root, *spec.Name)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// NetPlanの雛形へ書出す2
	err = LinuxSetup_createNetplan(spec, vm_root)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// hostidの書き出し
	err = LinuxSetup_hostid(spec, vm_root)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// 後始末
	err = UnMountLocal(*spec.Uuid)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	err = KpartOff(vg, oslv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// 正常終了
	return nil
}

func ConfigRootVol2(spec api.VmSpec, vg string, oslv string) error {
	err := KpartOn(vg, oslv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	// マウント
	vm_root, err := MountLocal(vg, oslv, *spec.Uuid)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// ホスト名の書き込み
	err = LinuxSetup_hostname(vm_root, *spec.Name)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// NetPlanの雛形へ書出す2
	err = LinuxSetup_createNetplan2(spec, vm_root)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// hostidの書き出し
	err = LinuxSetup_hostid2(spec, vm_root)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// 後始末
	err = UnMountLocal(*spec.Uuid)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	err = KpartOff(vg, oslv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// 正常終了
	return nil
}
