package dns

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net"
	"time"
)

// ヘルパー関数 ローカルのCoreDNSでアドレス解決する
func resolv_by_localdns(dn string) ([]string, error) {
	// https://pkg.go.dev/net#Resolver
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: time.Millisecond * time.Duration(10000),
			}
			return d.DialContext(ctx, "udp", "127.0.0.1:1053")
		},
	}
	return r.LookupHost(context.Background(), dn)
}

var _ = Describe("Etcd", Ordered, func() {

	var url string
	var dn1, dn2, dn3 string
	var cc *CoreDNSClient

	BeforeAll(func() {
		url = "http://127.0.0.1:2379"
		dn1 = "server1.a.labo.local"
		dn2 = "server2.a.labo.local"
		dn3 = "server3.a.labo.local"
		cc, _ = NewCdnsEp(url)
	})

	AfterAll(func() {
		//err := os.Clearenv("WEIGHT_UNITS")
	})

	Describe("ETCD and CoreDNS Test", func() {
		Context("Access Test for ETCD", func() {
			It("Add a record", func() {
				err := cc.Add(DnsRecord{
					Hostname: dn1,
					Ipv4:     "192.168.10.1",
					Ttl:      60,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			It("Get a record by Key(FQDN)", func() {
				dnsname := dn1
				rec, err := cc.Get(DnsRecord{Hostname: dnsname})
				GinkgoWriter.Println("Get() = ", rec)
				Expect(err).NotTo(HaveOccurred())
				Expect(rec.Host).To(Equal("192.168.10.1"))
				Expect(rec.Ttl).To(Equal(uint64(60)))
			})
			It("Update a record by Key(FQDN)", func() {
				err := cc.Add(DnsRecord{
					Hostname: dn1,
					Ipv4:     "192.168.10.2",
					Ttl:      90,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			It("Verify updated a record by Key(FQDN)", func() {
				rec, err := cc.Get(DnsRecord{Hostname: dn1})
				GinkgoWriter.Println("Get() = ", rec)
				Expect(err).NotTo(HaveOccurred())
				Expect(rec.Host).To(Equal("192.168.10.2"))
				Expect(rec.Ttl).To(Equal(uint64(90)))
			})
			It("Add new record", func() {
				err := cc.Add(DnsRecord{
					Hostname: dn2,
					Ipv4:     "192.168.10.2",
					Ttl:      90,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			It("Verify added the record", func() {
				rec, err := cc.Get(DnsRecord{Hostname: dn2})
				Expect(err).NotTo(HaveOccurred())
				Expect(rec.Host).To(Equal("192.168.10.2"))
				Expect(rec.Ttl).To(Equal(uint64(90)))
				GinkgoWriter.Println("Get() = ", rec)
			})

			It("Delete the record #1", func() {
				err := cc.Del(DnsRecord{Hostname: dn1})
				Expect(err).NotTo(HaveOccurred())
			})
			It("Verify deleted record #1", func() {
				_, err := cc.Get(DnsRecord{Hostname: dn1})
				Expect(err).To(HaveOccurred())
			})

			It("Delete the record #2", func() {
				err := cc.Del(DnsRecord{Hostname: dn2})
				Expect(err).NotTo(HaveOccurred())
			})
			It("Verify deleted record #2", func() {
				_, err := cc.Get(DnsRecord{Hostname: dn2})
				Expect(err).To(HaveOccurred())
			})

			It("Delete a no-existing record #3", func() {
				err := cc.Del(DnsRecord{Hostname: dn3})
				Expect(err).NotTo(HaveOccurred())
			})

		})

		Context("CoreDNS access test", func() {
			It("Resolve local existing entry", func() {
				ip, err := resolv_by_localdns("minio.labo.local")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("ip = ", ip)
			})
			It("Resolve public web site", func() {
				ip, err := resolv_by_localdns("www.google.com")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("ip = ", ip)
			})
		})

		Context("Relation test CoreDNS and ETCD", func() {
			It("Add new record", func() {
				err := cc.Add(DnsRecord{
					Hostname: dn1,
					Ipv4:     "192.168.20.1",
					Ttl:      30,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			It("Resolve added entry", func() {
				ip, err := resolv_by_localdns(dn1)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("ip = ", ip[0])
			})
		})
	})
})
