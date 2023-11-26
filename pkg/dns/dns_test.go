package dns

import (
	"fmt"
	"testing"
	"net"
	"context"
	"time"
)

const dburl = "http://127.0.0.1:2379"

// ETCD登録
func TestAdd(t *testing.T) {
	fmt.Println("case 1")
	var d DnsRecord
	d.Hostname = "server1.a.labo.local"
	d.Ipv4     = "192.168.10.1"
	d.Ttl      = 60

	err := Add(d, dburl)
	if err != nil {
		t.Errorf("Add() %v", err)
	}
}

// ETCD キーは、なんだっけ？
func TestGet(t *testing.T) {
	fmt.Println("case 2")
	var d DnsRecord
	d.Hostname = "server1.a.labo.local"

	r, err := Get(d, dburl)
	if err != nil {
		t.Errorf("Add() %v", err)
	}
}

// ヘルパー関数 ローカルのDNSでアドレス解決する
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


func TestResolve(t *testing.T) {
	fmt.Println("case 3")
	ip, err := resolv_by_localdns("server1.a.labo.local")
    if err != nil {
		t.Errorf("net.ResolveIPAddr() %v", err)
    }
	fmt.Println("Resovle addr is ", ip[0])
}


func TestAdd_without_topdomain(t *testing.T) {
	fmt.Println("case 4")
	var d DnsRecord
	d.Hostname = "server1.test"
	err := Add(d, dburl)
	if err == nil {
		t.Errorf("Add() %v", err)
	}
}

func TestAdd_without_domain(t *testing.T) {
	fmt.Println("case 4")
	var d DnsRecord
	d.Hostname = "server1"

	err := Add(d, dburl)
	if err == nil {
		t.Errorf("Add() %v", err)
	}
}

func TestAdd2(t *testing.T) {
	fmt.Println("case 5")
	var d DnsRecord
	d.Hostname = ""
	d.Ipv4     = "192.168.10.1"
	d.Ttl      = 60

	err := Add(d, dburl)
	if err == nil {
		t.Errorf("Add() %v", err)
	}
}

func TestAdd3(t *testing.T) {
	fmt.Println("case 5")
	var d DnsRecord
	d.Hostname = "server1.test.local"
	d.Ipv4 = ""
	d.Ttl  = 60

	err := Add(d, dburl)
	if err == nil {
		t.Errorf("Add() %v", err)
	}
}

/*
func TestDel(t *testing.T) {
	fmt.Println("Case 5")

	var d DnsRecord
	d.Hostname = "server1.test.local"
	d.Ipv4     = "192.168.10.1"
    d.Ttl      = 60
	d.Path     = "test"

	err := Del(d, dburl)
	if err != nil {
		t.Errorf("Del() %v", err)
	}
}
*/

/*
func TestDel2(t *testing.T) {
	fmt.Println("Case 6")

	var d DnsRecord
	d.Hostname = ""
	d.Ipv4     = "192.168.10.1"
	d.Ttl      = 60
	d.Path     = "test"

	err := Del(d, dburl)
	if err == nil {
		t.Errorf("Del() %v", err)
	}
}
*/