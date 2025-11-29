package config

/*
	クラスタと仮想サーバー設定用YAMLの定義
*/

// Storage defines model for storage.
type Storage struct {
	Lv   *string `yaml:"lv,omitempty"`
	Name *string `yaml:"name,omitempty"`
	Path *string `yaml:"path,omitempty"`
	Size *int64  `yaml:"size,omitempty"`
	Type *string `yaml:"type,omitempty"`
	Vg   *string `yaml:"vg,omitempty"`
}

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
