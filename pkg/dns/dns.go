package dns

import (
	"errors"
	"log"
	"os"
	"strconv"
	"strings"

	db "github.com/takara9/marmot/pkg/db"
)

func init() {
	logfile, _ := os.Create("error.log")
	log.SetOutput(logfile)

}

func checkParam(rec DnsRecord) error {
	// check param
	if len(rec.Hostdomain) == 0 {
		return errors.New("must set a server name ")
	}
	if len(rec.Ipaddr_v4) == 0 {
		return errors.New("must set a ipaddr")
	}
	if len(rec.RootPath) == 0 {
		return errors.New("must set a RootPath for Etcd")
	}

	// port no

	return nil
}

// Make etcd path
func convertEtcdPath(Hostdomain string) (string, error) {

	h := strings.Split(Hostdomain, ".")
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

	path, err := convertEtcdPath(rec.Hostdomain)
	if err != nil {
		return err
	}

	path = "/" + rec.RootPath + path

	log.Println("etcd path = ", path)

	// Create JSON value
	var ent db.DNSEntry
	ent.Host = rec.Ipaddr_v4
	ent.Port, _ = strconv.Atoi(rec.PortNo)

	con, err := db.Connect(dbUrl)
	if err != nil {
		log.Println("db.Connect()", " ", err)
		return err
	}

	// Add etcd
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

	path, err := convertEtcdPath(rec.Hostdomain)
	if err != nil {
		return d, err
	}
	path = "/" + rec.RootPath + path
	log.Println("etcd path = ", path)

	con, err := db.Connect(dbUrl)
	if err != nil {
		log.Println("db.Connect()", " ", err)
		return d, err
	}

	// Add etcd
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

	path, err := convertEtcdPath(rec.Hostdomain)
	if err != nil {
		return err
	}
	path = "/" + rec.RootPath + path
	log.Println("etcd path = ", path)

	// Create JSON value
	var ent db.DNSEntry
	ent.Host = rec.Ipaddr_v4
	ent.Port, _ = strconv.Atoi(rec.PortNo)

	con, err := db.Connect(dbUrl)
	if err != nil {
		log.Println("db.Connect()", " ", err)
		return err
	}

	// Add etcd
	err = db.DelByKey(con, path)
	if err != nil {
		return err
	}

	return nil
}
