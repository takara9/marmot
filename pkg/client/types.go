package client

import (
	"time"

	"github.com/google/uuid"
)

// VM Storage
type Storage struct {
	Name string
	Size int
	Path string
	Lv   string // Logical Volume
	Vg   string // Volume Group
}

type VirtualMachine struct {
	Name        string    // OS Hostname
	ClusterName string    // クラスタ名
	Key         string    // アサイン時に割当 接頭語VM + シリアル番号 etc キー
	Uuid        uuid.UUID // アサイン時に決定
	HvNode      string    // アサイン時に決定
	HvPort      int       // アサイン時に決定
	Ctime       time.Time // Unix時刻で登録タイムスタンプ
	Stime       time.Time //           実行開始時間
	Status      int       // 0 登録中、1 プロビ中、2 実行中、3 停止中、4 削除中、5 Error
	Cpu         int       // given condition VCPUの数
	Memory      int       // given condition メモリ量 GB
	PrivateIp   string    // given condition
	PublicIp    string    // given condition
	Storage     []Storage // given condition
	Playbook    string    // given condition
	Comment     string    // given condition
	OsLv        string    // OS Disk Logical Volume
	OsVg        string    // OS Disk Volume Group
	OsVariant   string    // OS Variant
}

type StoragePool struct {
	VolGroup string // ボリュームグループ名
	FreeCap  uint64 // 空き容量 GB
	VgCap    uint64 // VM用ストレージ容量 GB
	Type     string // ストレージの種類　HDD, SSD, NVME
}

type Hypervisor struct {
	Nodename   string        // ハイパーバイザーノード名
	Port       int           // HVコントローラー(marmotd)のポート番号
	Cpu        int           // 搭載CPU量 仮想コア数＝VCPU
	Memory     int           // 搭載メモリ量 MB
	IpAddr     string        // ハイパーバイザーホストのIP
	FreeCpu    int           // 空きCPU
	FreeMemory int           // 空きメモリ
	Key        string        // Etcd Key
	Status     int           // 状態  0:停止中, 1:障害中, 2:稼働中
	StgPool    []StoragePool //
}

type Hypervisors struct {
	Hvs []Hypervisor
}