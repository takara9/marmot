package volumes

import (
	"errors"
)

//ボリューム生成,API, CreateVolume()
/*
	qcow2形式、lvm形式、raw形式のいずれかでボリュームを作成する。
	パラメータとして、ボリュームタイプ、サイズ、その他オプションを指定する。
	成功した場合は新規ボリュームのIDを返す。
*/
func CreateVolume() error {
	// データベースにボリューム情報を登録


	// ボリュームの実体を作成
	//タイプで分岐


	// データベースの状態を更新

	return errors.New("Not implemented")
}

// ボリューム削除,API, RemoveVolume(volId)
func RemoveVolume(volId string) error {
	return errors.New("Not implemented")
}

// ボリュームのリスト,API, GetVolumes(volType)
func GetVolumes(volType string) ([]string, error) {
	return nil, errors.New("Not implemented")
}

// ボリュームの拡張,API, ExpandVolume(volId, newSize)
func ExpandVolume(volId string, newSize int) error {
	return errors.New("Not implemented")
}



// ボリュームの仮想マシンへのアタッチとデタッチ,API, AttachVol(vmId, volId), DetachVol(vmId, volId)
func AttachVol(vmId string, volId string) error {
	return errors.New("Not implemented")
}

func DetachVol(vmId string, volId string) error {
	return errors.New("Not implemented")
}

// ボリュームの複製,API, CopyVolume(volId)
func CopyVolume(volId string) error {
	return errors.New("Not implemented")
}
