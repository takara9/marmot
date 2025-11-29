package types

// Serial Number Control
type VmSerial struct {
	Serial uint64
	Start  uint64
	Step   uint64
	Key    string
}
type OsImageTemplate struct {
	LogicalVolume string // vg1
	VolumeGroup string // lv01,lv02
	OsVariant   string // Key
}

const (
	INITALIZING  = 0 // 0 登録中
	PROVISIONING = 1 // 1 プロビ中
	RUNNING      = 2 // 2 実行中
	STOPPED      = 3 // 3 停止中
	DELETEING    = 4 // 4 削除中
	ERROR        = 5 // 5 エラー
)

type DNSEntry struct {
	Host string `json:"host"`
	Ttl  uint64 `json:"ttl"`
}
