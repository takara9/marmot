package db

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/types"
	etcd "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

const (
	HvPrefix              = "/marmot/hypervisor"
	VmPrefix              = "/marmot/virtualmachine"
	OsTemplateImagePrefix = "/marmot/osTemplateImage"
	OsImagePrefix         = "/marmot/osimage"
	VolumePrefix          = "/marmot/volume"
	SeqPrefix             = "/marmot/sequence"
	VersionKey            = "/marmot/version"
	JobPrefix             = "/marmot/job"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrUpdateConflict = errors.New("update conflict")
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
	d.Mutex = concurrency.NewMutex(d.Session, lockName)

	if err := d.Mutex.Lock(d.Ctx); err != nil {
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
	byteData, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("json marshal failed: %w", err)
	}
	_, err = d.Cli.Put(d.Ctx, key, string(byteData))
	if err != nil {
		return fmt.Errorf("etcd put failed: %w", err)
	}
	return nil
}

// 生データ取得
func (d *Database) GetRaw(key string) (*etcd.GetResponse, error) {
	resp, err := d.Cli.Get(d.Ctx, key, etcd.WithLimit(1))
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
	resp, err := d.GetRaw(key)
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
	resp, err := d.Cli.Get(d.Ctx, prefix, etcd.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd get prefix failed: %w", err)
	}
	if resp.Count == 0 {
		slog.Debug("GetDataByPrefix(): no results", "key", prefix)
		return nil, ErrNotFound
	}

	return resp, nil
}

// PutJSONCAS: ModRevision が expectedRev の時だけ更新する
func (d *Database) PutJSONCAS(key string, expectedRev int64, v interface{}) error {
	//ctx, cancel := context.WithTimeout(d.Ctx, 5*time.Second)
	//defer cancel()

	byteData, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("json marshal failed: %w", err)
	}

	//txn := d.Cli.Txn(ctx)
	txn := d.Cli.Txn(d.Ctx)
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

//---------------------------------------------------------------------------------------------------

// キーが一致した値を取得
/*
func (d *Database) _GetDataByKey(key string) ([]byte, error) {
	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)
	resp, err := d.Cli.Get(ctx, key, etcd.WithLimit(1))
	if err != nil {
		return nil, err
	}

	if resp.Count == 0 {
		return nil, ErrNotFound
	}

	return resp.Kvs[0].Value, nil
}

// 前方一致のサーチ
func (d *Database) GetDataByPrefix(key string) (*etcd.GetResponse, error) {
	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)
	resp, err := d.Cli.Get(ctx, key, etcd.WithPrefix())
	if err != nil {
		slog.Debug("GetDataByPrefix()", "err", err, "key", key)
		return nil, err
	}

	if resp.Count == 0 {
		slog.Debug("GetDataByPrefix(): no results", "key", key)
		return nil, ErrNotFound
	}

	return resp, nil
}
*/

// 廃止予定
func (d *Database) GetDnsByKey(path string) (types.DNSEntry, error) {
	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)
	var entry types.DNSEntry
	resp, err := d.Cli.Get(ctx, path)
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

/*
// 削除 キーに一致したデータ
func (d *Database) DeleteDataByKey(key string) error {
	mutex := concurrency.NewMutex(d.Session, "/lock/put/"+key)
	if err := mutex.Lock(d.Ctx); err != nil {
		if errors.Is(err, rpctypes.ErrLeaseNotFound) {
			slog.Debug("lease not found, ignoring")
		} else {
			return fmt.Errorf("failed to acquire etcd lock: %w", err)
		}
	}
	defer func() {
		if err := mutex.Unlock(d.Ctx); err != nil {
			slog.Error("failed to release lock", "err", err.Error())
		}
	}()
	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)
	_, err := d.Cli.Delete(ctx, key)
	if err != nil {
		slog.Error("DelDataByKey() failed", "err", err, "key", key)
	}
	return err
}

// etcdへ保存
func (d *Database) _PutDataEtcd(key string, v interface{}) error {
	mutex := concurrency.NewMutex(d.Session, "/lock/put/"+key)
	if err := mutex.Lock(d.Ctx); err != nil {
		if errors.Is(err, rpctypes.ErrLeaseNotFound) {
			slog.Debug("lease not found, ignoring")
		} else {
			return fmt.Errorf("failed to acquire etcd lock: %w", err)
		}
	}
	defer mutex.Unlock(d.Ctx)
	byteJSON, err := json.Marshal(v)
	if err != nil {
		slog.Error("failed to marshal data", "err", err, "key", key)
		return err
	}

	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)
	_, err = d.Cli.Put(ctx, key, string(byteJSON))
	if err != nil {
		slog.Error("failed to put data", "err", err, "key", key)
		return err
	}
	return nil
}
*/

// パブリックIPアドレスが一致するインスタンスを探す
func (d *Database) FindByPublicIPaddress(ipAddress string) (bool, error) {
	resp, err := d.GetByPrefix(VmPrefix)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}

	for _, ev := range resp.Kvs {
		var vm api.VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			return false, nil /// 例外的にエラーを無視
		}
		if ipAddress == *vm.PublicIp {
			return true, nil
		}
	}
	return false, nil
}

// プライベートIPアドレスが一致するインスンスを探す
func (d *Database) FindByPrivateIPaddress(ipAddress string) (bool, error) {
	resp, err := d.GetByPrefix(VmPrefix)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	for _, ev := range resp.Kvs {
		var vm api.VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			return false, nil /// 例外的にエラーを無視
		}
		if ipAddress == *vm.PrivateIp {
			return true, nil
		}
	}
	return false, nil
}

// ホスト名からVMキーを探す
func (d *Database) FindByHostname(hostname string) (string, error) {
	resp, err := d.GetByPrefix(VmPrefix)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", nil
		}
		return "", err
	}

	for _, ev := range resp.Kvs {
		var vm api.VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			return "", err
		}
		if hostname == vm.Name {
			return *vm.Key, err
		}
	}
	return "", errors.New("NotFound")
}

// ホスト名とクラスタ名でVMキーを取得する
func (d *Database) FindByHostAndClusteName(hostname string, clustername string) (string, error) {
	resp, err := d.GetByPrefix(VmPrefix)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", nil
		}
		slog.Error("FindByHostAndClusteName()", "err", err, "hostname", hostname, "clustername", clustername)
		return "", err
	}

	for _, ev := range resp.Kvs {
		var vm api.VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			slog.Error("FindByHostAndClusteName()", "err", err, "hostname", hostname, "clustername", clustername)
			return "", err
		}

		if hostname == vm.Name && clustername == *vm.ClusterName {
			return *vm.Key, nil
		}
	}
	return "", errors.New("NotFound")
}

func decodeBase64(data []byte) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, err
	}
	return []byte(decoded), nil
}
