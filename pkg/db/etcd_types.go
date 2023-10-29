package db

import (
	"time"

	"github.com/google/uuid"
)

type StoragePool struct {
	VolGroup string // ボリュームグループ名
	FreeCap  uint64 // 空き容量 GB
	VgCap    uint64 // VM用ストレージ容量 GB
	Type     string // ストレージの種類　HDD, SSD, NVME
}

type Hypervisor struct {
	Nodename   string        // ハイパーバイザーノード名
	Cpu        int           // 搭載CPU量 仮想コア数＝VCPU
	Memory     int           // 搭載メモリ量 MB
	IpAddr     string        // ハイパーバイザーホストのIP
	FreeCpu    int           // 空きCPU
	FreeMemory int           // 空きメモリ
	Key        string        // Etcd Key
	Status     int           // 状態  0:停止中, 1:障害中, 2:稼働中
	StgPool    []StoragePool //
	// 以下は廃止予定
	//FreeCap     uint64   // 空き容量 GB
	//VgCap       uint64   // VM用ストレージ容量 GB
	//VolGroups   []string // ボリュームグループのリスト
}

type Hypervisors struct {
	Hvs []Hypervisor
}

// Serial Number Control
type VmSerial struct {
	Serial uint64
	Start  uint64
	Step   uint64
	Key    string
}

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
	Key         string    // 　アサイン時に割当 接頭語VM + シリアル番号 etc キー
	Uuid        uuid.UUID // アサイン時に決定
	HvNode      string    // アサイン時に決定
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

type VirtualMachines struct {
	Vms []VirtualMachine
}

type OsImageTemplate struct {
	LogicaVol   string // vg1
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
	Port int    `json:"port"`
}

////////////////////////////////////
/*
type StgPool_yaml struct {
	VolGroup string        `yaml:"vg"`
	Type     string        `yaml:"type"`
}

// ハイパーバイザー
type Hypervisor_yaml struct {
	Name    string         `yaml:"name"`
	Cpu     uint64         `yaml:"cpu"`
	CpuFree uint64         `yaml:"free_cpu"`
	Ram     uint64         `yaml:"ram"`
	RamFree uint64         `yaml:"free_ram"`
	IpAddr  string         `yaml:"ip_addr"`
	Storage []StgPool_yaml `yaml:"storage_pool"`
}

type Hypervisors_yaml struct {
	Hvs  []Hypervisor_yaml `yaml:"hv_spec"`
	Imgs []Image_yaml      `yaml:"image_template"`
	Seq  []SeqNo_yaml      `yaml:"seqno"`
}

// OSイメージ　テンプレート
type Image_yaml struct {
	Name          string   `yaml:"name"`
	VolumeGroup   string   `yaml:"volumegroup"`
	LogicalVolume string   `yaml:"logicalvolume"`
}

// シーケンス番号
type SeqNo_yaml struct {
	Start uint64 `yaml:"start"`
	Step  uint64 `yaml:"step"`
	Key   string `yaml:"name"`
}
*/