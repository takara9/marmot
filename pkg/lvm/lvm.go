package lvm

import (
	tlvm "github.com/takara9/lvm" //独自機能拡張してあるので、go get ... を実行すること
)

// 論理ボリュームの存在チェック
func IsExist(vgx string, lvx string) error {
	tlvm.Verbose = false

	vg, err := tlvm.LookupVolumeGroup(vgx)
	if err != nil {
		return err
	}

	_, err = vg.LookupLogicalVolume(lvx)
	if err != nil {
		return err
	}

	return nil
}

// 論理ボリュームの作成
func CreateLV(vgx string, lvx string, sizeInByte uint64) error {
	tlvm.Verbose = false

	vg, err := tlvm.LookupVolumeGroup(vgx)
	if err != nil {
		return err
	}

	_, err = vg.CreateLogicalVolume(lvx, sizeInByte, nil)
	if err != nil {
		return err
	}
	return nil
}

// 論理ボリュームの削除
func RemoveLV(vgx string, lvx string) error {
	tlvm.Verbose = false

	vg, err := tlvm.LookupVolumeGroup(vgx)
	if err != nil {
		return err
	}

	lv, err := vg.LookupLogicalVolume(lvx)
	if err != nil {
		return err
	}

	err = lv.Remove()
	if err != nil {
		return err
	}
	return nil
}

// スナップショットの作成、OSボリューム作成用
func CreateSnapshot(vgx string, lvx string, svx string, sizeInByte uint64) error {
	tlvm.Verbose = false

	tags := []string{"snapshot", "marmot"}
	vg, err := tlvm.LookupVolumeGroup(vgx)
	if err != nil {
		return err
	}

	_, err = vg.CreateLogicalVolumeSnapshot(svx, sizeInByte, tags, lvx)
	if err == nil {
		return err
	}

	return nil
}

// ボリュームグループの総量量と空きチェック
func CheckVG(vgx string) (uint64, uint64, error) {
	tlvm.Verbose = false

	vg, err := tlvm.LookupVolumeGroup(vgx)
	if err != nil {
		return 0, 0, err
	}
	total_sz, free_sz, err := vg.CheckVg()
	if err != nil {
		return 0, 0, err
	}
	return total_sz, free_sz, err
}
