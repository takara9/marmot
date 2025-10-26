package config

import (
	"time"
)

// Error defines model for Error.
type Error struct {
	Code    int32  `yaml:"code"`
	Message string `yaml:"message"`
}

// Hypervisor defines model for Hypervisor.
type Hypervisor struct {
	Cpu        int32          `yaml:"cpu"`
	FreeCpu    *int32         `yaml:"freeCpu,omitempty"`
	FreeMemory *int64         `yaml:"freeMemory,omitempty"`
	IpAddr     *string        `yaml:"ipAddr,omitempty"`
	Key        *string        `yaml:"key,omitempty"`
	Memory     *int64         `yaml:"memory,omitempty"`
	NodeName   string         `yaml:"nodeName"`
	Port       *int32         `yaml:"port,omitempty"`
	Status     *int32         `yaml:"status,omitempty"`
	StgPool    *[]StoragePool `yaml:"stgPool,omitempty"`
}

// Hypervisors defines model for Hypervisors.
type Hypervisors = []Hypervisor

// MarmotConfig defines model for MarmotConfig.
type MarmotConfig struct {
	ClusterName       *string   `yaml:"cluster_name,omitempty"`
	Domain            *string   `yaml:"domain,omitempty"`
	Hypervisor        *string   `yaml:"hypervisor,omitempty"`
	ImageDefaultPath  *string   `yaml:"image_default_path,omitempty"`
	ImgaeTemplatePath *string   `yaml:"imgae_template_path,omitempty"`
	NetDevDefault     *string   `yaml:"net_dev_default,omitempty"`
	NetDevPrivate     *string   `yaml:"net_dev_private,omitempty"`
	NetDevPublic      *string   `yaml:"net_dev_public,omitempty"`
	OsVariant         *string   `yaml:"os_variant,omitempty"`
	PrivateIpSubnet   *string   `yaml:"private_ip_subnet,omitempty"`
	PublicIpDns       *string   `yaml:"public_ip_dns,omitempty"`
	PublicIpGw        *string   `yaml:"public_ip_gw,omitempty"`
	PublicIpSubnet    *string   `yaml:"public_ip_subnet,omitempty"`
	Qcow2Image        *string   `yaml:"qcow2_image,omitempty"`
	VmSpec            *[]VmSpec `yaml:"vm_spec,omitempty"`
}

// Pong defines model for Pong.
type Pong struct {
	Ping string `yaml:"ping"`
}

// ReplyMessage defines model for ReplyMessage.
type ReplyMessage struct {
	Message string `yaml:"message"`
}

// Version defines model for Version.
type Version struct {
	Version string `yaml:"version"`
}

// Storage defines model for storage.
type Storage struct {
	Lv   *string `yaml:"lv,omitempty"`
	Name *string `yaml:"name,omitempty"`
	Path *string `yaml:"path,omitempty"`
	Size *int64  `yaml:"size,omitempty"`
	Type *string `yaml:"type,omitempty"`
	Vg   *string `yaml:"vg,omitempty"`
}

// StoragePool defines model for storagePool.
type StoragePool struct {
	FreeCap  *int64  `yaml:"freeCap,omitempty"`
	Type     *string `yaml:"type,omitempty"`
	VgCap    *int64  `yaml:"vgCap,omitempty"`
	VolGroup *string `yaml:"volGroup,omitempty"`
}

// VirtualMachine defines model for virtualMachine.
type VirtualMachine struct {
	HvIpAddr    *string    `yaml:"HvIpAddr,omitempty"`
	HvNode      string     `yaml:"HvNode"`
	HvPort      *int32     `yaml:"HvPort,omitempty"`
	CTime       *time.Time `yaml:"cTime,omitempty"`
	ClusterName *string    `yaml:"clusterName,omitempty"`
	Comment     *string    `yaml:"comment,omitempty"`
	Cpu         *int32     `yaml:"cpu,omitempty"`
	Key         *string    `yaml:"key,omitempty"`
	Memory      *int64     `yaml:"memory,omitempty"`
	Name        string     `yaml:"name"`
	OsLv        *string    `yaml:"osLv,omitempty"`
	OsVariant   *string    `yaml:"osVariant,omitempty"`
	OsVg        *string    `yaml:"osVg,omitempty"`
	Playbook    *string    `yaml:"playbook,omitempty"`
	PrivateIp   *string    `yaml:"privateIp,omitempty"`
	PublicIp    *string    `yaml:"publicIp,omitempty"`
	STime       *time.Time `yaml:"sTime,omitempty"`
	Status      *int32     `yaml:"status,omitempty"`
	Storage     *[]Storage `yaml:"storage,omitempty"`
	Uuid        *string    `yaml:"uuid,omitempty"`
}

// VirtualMachines defines model for virtualMachines.
type VirtualMachines = []VirtualMachine

// VmSpec defines model for vmSpec.
type VmSpec struct {
	Comment       *string    `yaml:"comment,omitempty"`
	Cpu           *int32     `yaml:"cpu,omitempty"`
	Key           *string    `yaml:"key,omitempty"`
	Memory        *int64     `yaml:"memory,omitempty"`
	Name          *string    `yaml:"name,omitempty"`
	Ostemplv      *string    `yaml:"ostemplv,omitempty"`
	Ostempvariant *string    `yaml:"ostempvariant,omitempty"`
	Ostempvg      *string    `yaml:"ostempvg,omitempty"`
	Playbook      *string    `yaml:"playbook,omitempty"`
	PrivateIp     *string    `yaml:"private_ip,omitempty"`
	PublicIp      *string    `yaml:"public_ip,omitempty"`
	Storage       *[]Storage `yaml:"storage,omitempty"`
	Uuid          *string    `yaml:"uuid,omitempty"`
}
