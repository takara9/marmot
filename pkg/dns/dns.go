package dns

import (
	"errors"
	"log"
	"strings"
	db "github.com/takara9/marmot/pkg/db"
)

type DnsRecord struct {
	Hostname   string
	Ipv4       string
	Ttl        uint64
}

// Check DNS record param
func checkParam(rec DnsRecord) error {
	if len(rec.Hostname) == 0 {
		return errors.New("must set server name ")
	}
	return nil
}


func checkParam2(rec DnsRecord) error {
	if len(rec.Ipv4) == 0 {
		return errors.New("must set IP address")
	}
	return nil
}

// Make etcd path
func convertEtcdPath(Hostname string) (string, error) {

	h := strings.Split(Hostname, ".")
	if len(h) < 3 {
		return "", errors.New("must set a hostname and domain")
	}

	var etcdpath string
	for i := len(h) - 1; i >= 0; i-- {
		etcdpath = etcdpath + "/" + h[i]
	}

	return etcdpath, nil
}

// 登録
func Add(rec DnsRecord, dbUrl string) error {

	err := checkParam(rec)
	if err != nil {
		return err
	}
	err = checkParam2(rec)
	if err != nil {
		return err
	}

	path, err := convertEtcdPath(rec.Hostname)
	if err != nil {
		return err
	}

	// Create JSON value
	var ent db.DNSEntry
	ent.Host = rec.Ipv4
	ent.Ttl  = rec.Ttl

	con, err := db.Connect(dbUrl)
	if err != nil {
		log.Println("db.Connect()", " ", err)
		return err
	}

	// Add etcd
	path = "/skydns" + path
	err = db.PutDataEtcd(con, path, &ent)
	if err != nil {
		return err
	}

	return nil
}

// 取得
func Get(rec DnsRecord, dbUrl string) (db.DNSEntry, error) {

	var d db.DNSEntry
	err := checkParam(rec)
	if err != nil {
		return d, err
	}

	path, err := convertEtcdPath(rec.Hostname)
	if err != nil {
		return d, err
	}

	con, err := db.Connect(dbUrl)
	if err != nil {
		log.Println("db.Connect()", " ", err)
		return d, err
	}

	// Get etcd
	path = "/skydns" + path
	rslt, err := db.GetEtcdByKey(con, path)
	if err != nil {
		return rslt, err
	}
	return rslt, err
}

// 削除
func Del(rec DnsRecord, dbUrl string) error {
	err := checkParam(rec)
	if err != nil {
		return err
	}

	path, err := convertEtcdPath(rec.Hostname)
	if err != nil {
		return err
	}

	// Create JSON value
	var ent db.DNSEntry
	ent.Host = rec.Ipv4
	ent.Ttl  = rec.Ttl

	con, err := db.Connect(dbUrl)
	if err != nil {
		log.Println("db.Connect()", " ", err)
		return err
	}

	// Add etcd
	path = "/skydns" + path
	err = db.DelByKey(con, path)
	if err != nil {
		return err
	}

	return nil
}
