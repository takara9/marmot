package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	etcd "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/types"
)

const (
	HvPrefix              = "/marmot/hypervisor"
	VmPrefix              = "/marmot/virtualmachine"
	OsTemplateImagePrefix = "/marmot/osTemplateImage"
	VolumePrefix          = "/marmot/volume"
	ServerPrefix          = "/marmot/server"
	SeqPrefix             = "/marmot/sequence"
	VersionKey            = "/marmot/version"
	JobPrefix             = "/marmot/job"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrUpdateConflict = errors.New("update conflict")
	ErrFound          = errors.New("found")
)

type Database struct {
	Cli     *etcd.Client
	Ctx     context.Context
	Session *concurrency.Session
	Mutex   *concurrency.Mutex
}

func NewDatabase(url string) (*Database, error) {
	ctx := context.Background()
	cli, err := etcd.New(etcd.Config{
		Endpoints:   []string{url},
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		slog.Error("failed to connect to etcd", "err", err)
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	// セッションの生成、分散ロックの有効化のため
	session, err := concurrency.NewSession(cli, concurrency.WithContext(ctx))
	if err != nil {
		cli.Close()
		slog.Error("failed to create etcd session", "err", err)
		return nil, fmt.Errorf("failed to create etcd session: %w", err)
	}

	return &Database{
		Cli:     cli,
		Ctx:     ctx,
		Session: session,
		Mutex:   nil,
	}, nil
}

func (d *Database) Close() error {
	if d.Session != nil {
		_ = d.Session.Close()
	}
	if d.Cli != nil {
		return d.Cli.Close()
	}
	return nil
}

// LockKey: 指定キーに対する分散ロックを取得
func (d *Database) LockKey(lockName string) (*concurrency.Mutex, error) {
	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)
	d.Mutex = concurrency.NewMutex(d.Session, lockName)
	if err := d.Mutex.Lock(ctx); err != nil {
		return nil, fmt.Errorf("failed to acquire lock %s: %w", lockName, err)
	}
	return d.Mutex, nil
}

// UnlockKey: エラーを無視してでも必ず呼ぶ想定
func (d *Database) UnlockKey(m *concurrency.Mutex) {
	_ = m.Unlock(d.Ctx)
}

// 単純 Put（ロック前提・上書きで良い場合）
func (d *Database) PutJSON(key string, v interface{}) error {
	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)
	byteJSON, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("json marshal failed: %w", err)
	}
	_, err = d.Cli.Put(ctx, key, string(byteJSON))
	if err != nil {
		return fmt.Errorf("etcd put failed: %w", err)
	}
	return nil
}

// 生データ取得
func (d *Database) getRaw(key string) (*etcd.GetResponse, error) {
	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)
	resp, err := d.Cli.Get(ctx, key, etcd.WithLimit(1))
	if err != nil {
		return nil, fmt.Errorf("etcd get failed: %w", err)
	}
	if resp.Count == 0 {
		return nil, ErrNotFound
	}
	return resp, nil
}

// JSONデコードして取得
func (d *Database) GetJSON(key string, out interface{}) (*etcd.GetResponse, error) {
	resp, err := d.getRaw(key)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(resp.Kvs[0].Value, out); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}
	return resp, nil
}

// プレフィックス検索で取得
func (d *Database) GetByPrefix(prefix string) (*etcd.GetResponse, error) {
	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)
	resp, err := d.Cli.Get(ctx, prefix, etcd.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd get prefix failed: %w", err)
	}
	if resp.Count == 0 {
		return nil, ErrNotFound
	}

	return resp, nil
}

// PutJSONCAS: ModRevision が expectedRev の時だけ更新する
func (d *Database) PutJSONCAS(key string, expectedRev int64, v interface{}) error {
	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)

	byteData, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("json marshal failed: %w", err)
	}

	txn := d.Cli.Txn(ctx)
	txnResp, err := txn.
		If(etcd.Compare(etcd.ModRevision(key), "=", expectedRev)).
		Then(etcd.OpPut(key, string(byteData))).
		Else(etcd.OpGet(key)).
		Commit()
	if err != nil {
		return fmt.Errorf("etcd txn failed: %w", err)
	}
	if !txnResp.Succeeded {
		return ErrUpdateConflict
	}
	return nil
}

func (d *Database) DeleteJSON(key string) error {
	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)
	_, err := d.Cli.Delete(ctx, key)
	if err != nil {
		slog.Error("DeleteJSON() failed", "err", err, "key", key)
		return err
	}
	return nil
}

// 削除対象
func (d *Database) GetDnsByKey(path string) (types.DNSEntry, error) {
	var entry types.DNSEntry
	resp, err := d.Cli.Get(d.Ctx, path)
	if err != nil {
		return entry, err
	}

	if resp.Count == 0 {
		return entry, errors.New("not found")
	}

	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &entry)
	if err != nil {
		return entry, err
	}
	return entry, nil
}

// パブリックIPアドレスが一致するインスタンスを探す
func (d *Database) FindByPublicIPaddress(ipAddress string) error {
	resp, err := d.GetByPrefix(VmPrefix)
	if err != nil {
		return err
	}

	for _, ev := range resp.Kvs {
		var vm api.VirtualMachine
		if err := json.Unmarshal([]byte(ev.Value), &vm); err != nil {
			slog.Error("failed unmarshal", "err", err, "ipAddress", ipAddress)
			return err
		}
		if ipAddress == *vm.PublicIp {
			return ErrFound
		}
	}
	return ErrNotFound
}

// プライベートIPアドレスが一致するインスンスを探す
func (d *Database) FindByPrivateIPaddress(ipAddress string) error {
	resp, err := d.GetByPrefix(VmPrefix)
	if err != nil {
		return err
	}

	for _, ev := range resp.Kvs {
		var vm api.VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			slog.Error("failed unmarshal", "err", err, "ipAddress", ipAddress)
			return err
		}
		if ipAddress == *vm.PrivateIp {
			return ErrFound
		}
	}
	return ErrNotFound
}

// ホスト名とクラスタ名でVMキーを取得する
func (d *Database) FindByHostAndClusteName(hostname string, clustername string) (string, error) {
	resp, err := d.GetByPrefix(VmPrefix)
	if err != nil {
		return "", err
	}
	for _, ev := range resp.Kvs {
		var vm api.VirtualMachine
		if err := json.Unmarshal([]byte(ev.Value), &vm); err != nil {
			slog.Error("FindByHostAndClusteName()", "err", err, "hostname", hostname, "clustername", clustername)
			return "", err
		}

		if hostname == vm.Name && clustername == *vm.ClusterName {
			return *vm.Key, ErrFound
		}
	}
	return "", ErrNotFound
<<<<<<< HEAD
}
=======
}
>>>>>>> origin/main
