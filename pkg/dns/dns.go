package dns

import (
	"errors"
	"log/slog"
	"strings"

	db "github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/types"
)

type DnsRecord struct {
	Hostname string
	Ipv4     string
	Ttl      uint64
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
		slog.Error("", "err", err)
		return err
	}

	err = checkParam2(rec)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	path, err := convertEtcdPath(rec.Hostname)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// Create JSON value
	var ent types.DNSEntry
	ent.Host = rec.Ipv4
	ent.Ttl = rec.Ttl

	d, err := db.NewDatabase(dbUrl)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// Add etcd
	path = "/skydns" + path
	err = d.PutJSON(path, &ent)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	return nil
}

// 取得
func Get(rec DnsRecord, dbUrl string) (types.DNSEntry, error) {
	var dd types.DNSEntry
	err := checkParam(rec)
	if err != nil {
		slog.Error("", "err", err)
		return dd, err
	}

	path, err := convertEtcdPath(rec.Hostname)
	if err != nil {
		slog.Error("", "err", err)
		return dd, err
	}

	d, err := db.NewDatabase(dbUrl)
	if err != nil {
		slog.Error("", "err", err)
		return dd, err
	}

	// Get etcd
	path = "/skydns" + path
	rslt, err := d.GetDnsByKey(path)
	if err != nil {
		slog.Error("", "err", err)
		return rslt, err
	}
	return rslt, err
}

// 削除
func Del(rec DnsRecord, dbUrl string) error {
	err := checkParam(rec)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	path, err := convertEtcdPath(rec.Hostname)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// Create JSON value
	var ent types.DNSEntry
	ent.Host = rec.Ipv4
	ent.Ttl = rec.Ttl

	d, err := db.NewDatabase(dbUrl)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// Add etcd
	path = "/skydns" + path
	err = d.DeleteJSON(path)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	return nil
}
