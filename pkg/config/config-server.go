package config

// Server defines model for Server.
type Server struct {
	Name       string     `yaml:"name"`
	Cpu        *int       `yaml:"cpu,omitempty"`
	Memory     *int       `yaml:"memory,omitempty"`
	OsVariant  *string    `yaml:"os_variant,omitempty"`
	BootVolume *Volume    `yaml:"boot_volume,omitempty"`
	Playbook   *string    `yaml:"playbook,omitempty"`
	Network    *[]Network `yaml:"network,omitempty"`
	Storage    *[]Volume  `yaml:"storage,omitempty"`
	Comment    *string    `yaml:"comment,omitempty"`
}

// Volume defines model for Volume.
type Volume struct {
	Name    string  `yaml:"name"`
	Size    *int    `yaml:"size,omitempty"`
	Comment *string `yaml:"comment,omitempty"`
	Type    *string `yaml:"type,omitempty"`
	Kind    *string `yaml:"kind,omitempty"`
}

type Route struct {
	Destination string  `yaml:"destination"`
	Gateway     string  `yaml:"gateway"`
	Comment     *string `yaml:"comment,omitempty"`
}

type Nameservers struct {
	Addresses *[]string `yaml:"addresses,omitempty"`
	Search    *[]string `yaml:"search,omitempty"`
}

type Network struct {
	Name        string       `yaml:"name"`
	Address     *string      `yaml:"address,omitempty"`
	Netmask     *string      `yaml:"netmask,omitempty"`
	Portgroup   *string      `yaml:"portgroup,omitempty"`
	Routes      *[]Route     `yaml:"routes,omitempty"`
	Nameservers *Nameservers `yaml:"nameservers,omitempty"`
	Vlans       *[]uint      `yaml:"vlans,omitempty"`
	Comment     *string      `yaml:"comment,omitempty"`
}
