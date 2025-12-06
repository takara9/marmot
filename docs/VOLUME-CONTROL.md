# ボリュームの構成と管理、VMへのアタッチとデタッチ、バックアップなど

## ゴール
- lvmだけでなく、qcow2など、ボリューム管理の選択肢を広げる
- 最終的にpkg/lvmを巻き取ることを目指す
    - func IsExist(vgx string, lvx string) error 論理ボリュームの存在チェック
    - func CreateLV(vgx string, lvx string, size uint64) error 論理ボリュームの作成
    - func RemoveLV(vgx string, lvx string) error 論理ボリュームの削除
    - func CreateSnapshot(vgx string, lvx string, svx string, size uint64) error スナップショットの作成、OSボリューム作成用
    - func CheckVG(vgx string) (uint64, uint64, error) ボリュームグループの総量量と空きチェック
- データベースの操作、ジョブの実行など、時間のかかるボリュームコピーなどの機能の管理も担う

## 求められる機能
- ボリューム生成,API, CreateVolume()
- ボリューム削除,API, RemoveVolume(volId)
- ボリュームのリスト,API, GetVolumes(volType)
- ボリュームの拡張,API, ExpandVolume(volId, newSize)
- ボリュームの仮想マシンへのアタッチとデタッチ,API, AttachVol(vmId, volId), DetachVol(vmId, volId)
- ボリュームの複製,API, CopyVolume(volId)
- OSイメージのテンプレートを使った仮想イメージの生成,API、ここではないね。
- 仮想マシンからOSイメージのテンプレートの生成,API, CreateImageTempVolFromVm(volId)、ここではないかも...





 

