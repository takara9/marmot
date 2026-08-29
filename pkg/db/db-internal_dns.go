package db

import (
	"log/slog"
	"strings"
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

// fqdnToInternalDNSKey は完全修飾ドメイン名(FQDN)を、pkg/internal-dns の
// DomainToMarmotPath と整合する階層キー(ラベルを逆順にして"/"で結合)に変換する。
func fqdnToInternalDNSKey(fqdn string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(fqdn), ".")
	labels := strings.Split(trimmed, ".")
	for i, j := 0, len(labels)-1; i < j; i, j = i+1, j-1 {
		labels[i], labels[j] = labels[j], labels[i]
	}
	return InternalDNSPrefix + "/" + strings.Join(labels, "/")
}

// PutDnsEntryFQDN は、多階層の完全修飾ドメイン名(例: サービス名.ネームスペース名.MKEクラスタ名.HVホスト名.labo.local)
// をキーとしてIPアドレスを登録する。内部DNSサーバーの問い合わせ処理(DomainToMarmotPaths)と
// 同じ階層構造で保存するため、PutDnsEntry(2階層専用)とは別に用意する。
func (d *Database) PutDnsEntryFQDN(fqdn, ipAddress string) error {
	slog.Debug("Putting DNS entry (FQDN)", "fqdn", fqdn, "ipAddress", ipAddress)

	key := fqdnToInternalDNSKey(fqdn)
	lockKey := "/lock/dns-fqdn/" + key
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	if err := d.PutJSON(key, ipAddress); err != nil {
		slog.Error("failed to write DNS entry", "err", err, "key", key)
		return err
	}
	return nil
}

// GetDnsEntryFQDN は、完全修飾ドメイン名からIPアドレスを取得する。
func (d *Database) GetDnsEntryFQDN(fqdn string) (string, error) {
	slog.Debug("Getting DNS entry (FQDN)", "fqdn", fqdn)

	var ipAddress string
	key := fqdnToInternalDNSKey(fqdn)
	if _, err := d.GetJSON(key, &ipAddress); err != nil {
		return "", err
	}
	return ipAddress, nil
}

// DeleteDnsEntryFQDN は、完全修飾ドメイン名でエントリーを削除する。
func (d *Database) DeleteDnsEntryFQDN(fqdn string) error {
	slog.Debug("Deleting DNS entry (FQDN)", "fqdn", fqdn)

	key := fqdnToInternalDNSKey(fqdn)
	lockKey := "/lock/dns-fqdn/" + key
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	if err := d.DeleteJSON(key); err != nil {
		slog.Error("failed to delete DNS entry", "err", err, "key", key)
		return err
	}
	return nil
}
