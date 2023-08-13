package dns

type DnsRecord struct {
	Hostdomain string
	Ipaddr_v4  string
	PortNo     string
	RootPath   string // Root Path in Etcd
}

type DNSEntry struct {
	Host string
	Port string
}
