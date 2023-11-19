package dns

import (
	"fmt"
	"testing"
)

const dburl = "http://127.0.0.1:2379"

func TestAdd(t *testing.T) {
	fmt.Println("case 1")
	var d DnsRecord
	d.Hostdomain = "server1.test.local"
	d.Ipaddr_v4 = "192.168.10.1"
	d.RootPath = "test"

	err := Add(d, dburl)
	if err != nil {
		t.Errorf("Add() %v", err)
	}
}

func TestGet(t *testing.T) {
	fmt.Println("case 1")
	var d DnsRecord
	d.Hostdomain = "server1.test.local"
	d.Ipaddr_v4 = "0.0.0.0"
	d.RootPath = "test"

	r, err := Get(d, dburl)
	fmt.Println("get result = ", r)

	if err != nil {
		t.Errorf("Add() %v", err)
	}
}

func TestAdd_without_topdomain(t *testing.T) {
	fmt.Println("case 2")
	var d DnsRecord
	d.Hostdomain = "server1.test"
	d.Ipaddr_v4 = "192.168.10.1"
	d.RootPath = "test"

	err := Add(d, dburl)
	if err == nil {
		t.Errorf("Add() %v", err)
	}
}

func TestAdd_without_domain(t *testing.T) {
	fmt.Println("case 3")
	var d DnsRecord
	d.Hostdomain = "server1"
	d.Ipaddr_v4 = "192.168.10.1"
	d.RootPath = "test"

	err := Add(d, dburl)
	if err == nil {
		t.Errorf("Add() %v", err)
	}
}

func TestAdd2(t *testing.T) {
	fmt.Println("case 4")
	var d DnsRecord
	d.Hostdomain = ""
	d.Ipaddr_v4 = "192.168.10.1"
	d.RootPath = "test"

	err := Add(d, dburl)
	if err == nil {
		t.Errorf("Add() %v", err)
	}
}

func TestAdd3(t *testing.T) {
	fmt.Println("case 5")
	var d DnsRecord
	d.Hostdomain = "server1.test.local"
	d.Ipaddr_v4 = ""
	d.RootPath = "test"

	err := Add(d, dburl)
	if err == nil {
		t.Errorf("Add() %v", err)
	}
}

func TestDel(t *testing.T) {
	fmt.Println("Case 5")

	var d DnsRecord
	d.Hostdomain = "server1.test.local"
	d.Ipaddr_v4 = "192.168.10.1"
	d.RootPath = "test"

	err := Del(d, dburl)
	if err != nil {
		t.Errorf("Del() %v", err)
	}
}

func TestDel2(t *testing.T) {
	fmt.Println("Case 6")

	var d DnsRecord
	d.Hostdomain = ""
	d.Ipaddr_v4 = "192.168.10.1"
	d.RootPath = "test"

	err := Del(d, dburl)
	if err == nil {
		t.Errorf("Del() %v", err)
	}
}
