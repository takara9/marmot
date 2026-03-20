package db

import (
	"log/slog"
)

// ホスト名、サブドメイン、IPアドレスを登録する
func (d *Database) PutDnsEntry(hostname, subdomain, ipAddress string) error {
	slog.Debug("Putting DNS entry", "hostname", hostname, "ipAddress", ipAddress)

	lockKey := "/lock/dns/" + subdomain + "/" + hostname
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	key := InternalDNSPrefix + "/" + subdomain + "/" + hostname
	if err := d.PutJSON(key, ipAddress); err != nil {
		slog.Error("failed to write image template data", "err", err, "key", key)
		return err
	}
	return nil
}

// ホスト名とドメイン名からIPアドレスを取得する
func (d *Database) GetDnsEntry(hostname, subdomain string) (string, error) {
	slog.Debug("Getting DNS entry", "hostname", hostname, "subdomain", subdomain)

	var ipAddress string
	key := InternalDNSPrefix + "/" + subdomain + "/" + hostname
	if _, err := d.GetJSON(key, &ipAddress); err != nil {
		return "", err
	}
	return ipAddress, nil
}

// ホスト名とドメイン名で、エントリーを削除する。
func (d *Database) DeleteDnsEntryByName(hostname, subdomain string) error {
	slog.Debug("Deleting DNS entry by name", "hostname", hostname)
	lockKey := "/lock/dns/" + subdomain + "/" + hostname
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)
	key := InternalDNSPrefix + "/" + subdomain + "/" + hostname
	if err := d.DeleteJSON(key); err != nil {
		slog.Error("failed to delete DNS entry", "err", err, "key", key)
		return err
	}
	return nil
}

/*
func (d *Database) UpdateDnsEntry(hostname string, ipAddress string) error {
	slog.Debug("Updateting DNS entry", "hostname", hostname, "ipAddress", ipAddress)
	// Here you would add the logic to put the DNS entry into etcd
	return nil
}
*/
