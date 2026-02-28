package config

import "time"

// Error defines model for Error.
type Error struct {
	Code    int32  `yaml:"code"`
	Message string `yaml:"message"`
}

// IPAddress defines model for IPAddress.
type IPAddress struct {
	HostId    *string `yaml:"HostId,omitempty"`
	IPAddress *string `yaml:"IPAddress,omitempty"`
	Netmask   *string `yaml:"Netmask,omitempty"`
	NetworkId *string `yaml:"NetworkId,omitempty"`
}

// IPNetwork defines model for IPNetwork.
type IPNetwork struct {
	AddressMaskLen   *string `yaml:"AddressMaskLen,omitempty"`
	EndAddress       *string `yaml:"EndAddress,omitempty"`
	Id               string  `yaml:"Id"`
	StartAddress     *string `yaml:"StartAddress,omitempty"`
	VirtualNetworkId *string `yaml:"virtualNetworkId,omitempty"`
}

// Metadata defines model for Metadata.
type Metadata struct {
	Comment      *string `yaml:"comment,omitempty"`
	Id           *string `yaml:"id,omitempty"`
	InstanceName *string `yaml:"instanceName,omitempty"`
	Key          *string `yaml:"key,omitempty"`
	Name         *string `yaml:"name,omitempty"`
	Uuid         *string `yaml:"uuid,omitempty"`
}

// Status defines model for Status.
type Status struct {
	CreationTimeStamp   *time.Time `yaml:"creationTimeStamp,omitempty"`
	DeletionTimeStamp   *time.Time `yaml:"deletionTimeStamp,omitempty"`
	LastUpdateTimeStamp *time.Time `yaml:"lastUpdateTimeStamp,omitempty"`
	Message             *string    `yaml:"message,omitempty"`
	Status              *int       `yaml:"status,omitempty"`
}

// VirtualNetwork defines model for VirtualNetwork.
type VirtualNetwork struct {
	Id       string              `yaml:"id"`
	Metadata *Metadata           `yaml:"Metadata,omitempty"`
	Spec     *VirtualNetworkSpec `yaml:"Spec,omitempty"`
	Status   *Status             `yaml:"Status,omitempty"`
}

// VirtualNetworkSpec defines model for VirtualNetworkSpec.
type VirtualNetworkSpec struct {
	BridgeName       *string `yaml:"bridgeName,omitempty"`
	Dhcp             *bool   `yaml:"dhcp,omitempty"`
	DhcpEndAddress   *string `yaml:"dhcpEndAddress,omitempty"`
	DhcpStartAddress *string `yaml:"dhcpStartAddress,omitempty"`
	ForwardMode      *string `yaml:"forwardMode,omitempty"`
	IpAddress        *string `yaml:"ipAddress,omitempty"`
	IpNetworkId      *string `yaml:"ipNetworkId,omitempty"`
	MacAddress       *string `yaml:"macAddress,omitempty"`
	Nat              *bool   `yaml:"nat,omitempty"`
	Netmask          *string `yaml:"netmask,omitempty"`
	Stp              *bool   `yaml:"stp,omitempty"`
}
