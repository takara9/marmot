package config

// Server defines model for Server.
type Server struct {
	Name      string    `yaml:"name"`
	Cpu       *int      `yaml:"cpu,omitempty"`
	Memory    *int      `yaml:"memory,omitempty"`
	OsVariant *string   `yaml:"osVariant,omitempty"`
	Playbook  *string   `yaml:"playbook,omitempty"`
	Nic       *[]Nic    `yaml:"nic,omitempty"`
	Storage   *[]Volume `yaml:"storage,omitempty"`
	Comment   *string   `yaml:"comment,omitempty"`
}

// Volume defines model for Volume.
type Volume struct {
	Name    string  `yaml:"name"`
	Size    *int    `yaml:"size,omitempty"`
	Comment *string `yaml:"comment,omitempty"`
}

type Nic struct {
	Name      string  `yaml:"name"`
	Network   *string `yaml:"network,omitempty"`
	IpAddress *string `yaml:"ipAddress,omitempty"`
	Comment   *string `yaml:"comment,omitempty"`
}
