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
	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
	etcd "go.etcd.io/etcd/client/v3"
	"log"
	"os"
	"os/exec"
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
	fmt.Println(cmd)
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

// OSテンプVolのスナップショットを作成してデバイス名を返す
// 　　サイズとテンプレートの選択が無い！？　　将来改良
func CreateOsLv(conn *etcd.Client, tempVg string, tempLv string) (string, error) {
	seq, err := db.GetSeq(conn, "LVOS")
	if err != nil {
		return "", err
	}
	lvName := fmt.Sprintf("oslv%04d", seq)
	var lvSize uint64 = 1024 * 1024 * 1024 * 16 // 8GB
	err = lvm.CreateSnapshot(tempVg, tempLv, lvName, lvSize)
	if err != nil {
		return "", err
	}
	return lvName, err
}

// データボリュームの作成
func CreateDataLv(conn *etcd.Client, sz uint64, vg string) (string, error) {
	seq, err := db.GetSeq(conn, "LVDATA")
	if err != nil {
		return "", err
	}
	lvName := fmt.Sprintf("data%04d", seq)
	lvSize := 1024 * 1024 * 1024 * sz
	err = lvm.CreateLV(vg, lvName, lvSize)
	if err != nil {
		return "", err
	}
	return lvName, err
}

// スナップショットボリュームをマウントして、 ホスト名とIPアドレスを設定
//
//	Ubuntu Linuxに限定
func ConfigRootVol(spec cf.VMSpec, vg string, oslv string) error {
	err := KpartOn(vg, oslv)
	if err != nil {
		log.Println("KpartOn()", err)
		return err
	}
	// マウント
	vm_root, err := MountLocal(vg, oslv, spec.Uuid)
	if err != nil {
		log.Println("MountLocal()", err)
		return err
	}

	// ホスト名の書き込み
	err = LinuxSetup_hostname(vm_root, spec.Name)
	if err != nil {
		log.Println("LinuxSetup_hostname()", err)
		return err
	}

	// NetPlanの雛形へ書出す2
	err = LinuxSetup_createNetplan(spec, vm_root)
	if err != nil {
		log.Println("LinuxSetup_createNetplan()", err)
		return err
	}

	// hostidの書き出し
	err = LinuxSetup_hostid(spec, vm_root)
	if err != nil {
		log.Println("LinuxSetup_hostid()", err)
		return err
	}

	// 後始末
	err = UnMountLocal(spec.Uuid)
	if err != nil {
		log.Println("UnMountLocal()", err)
		return err
	}

	err = KpartOff(vg, oslv)
	if err != nil {
		log.Println("KpartOff()", err)
		return err
	}

	// 正常終了
	return nil
}
