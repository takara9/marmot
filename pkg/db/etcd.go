package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	etcd "go.etcd.io/etcd/client/v3"

	. "github.com/takara9/marmot/pkg/types"
)

type Database struct {
	Cli *etcd.Client
	Ctx context.Context
}

func NewDatabase(url string) (*Database, error) {
	var db Database
	db.Ctx = context.Background()

	conn, err := etcd.New(etcd.Config{
		Endpoints:   []string{url},
		DialTimeout: 2 * time.Second,
	})
	db.Cli = conn
	return &db, err
}

// 前方一致のサーチ
func (d *Database) GetEtcdByPrefix(key string) (*etcd.GetResponse, error) {
	resp, err := d.Cli.Get(d.Ctx, key, etcd.WithPrefix())
	return resp, err
}

func (d *Database) GetEtcdByKey(path string) (DNSEntry, error) {

	var entry DNSEntry
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

// 削除 キーに一致したデータ
func (d *Database) DelByKey(key string) error {
	_, err := d.Cli.Delete(d.Ctx, key)
	return err
}

// etcdへ保存
func (d *Database) PutDataEtcd(k string, v interface{}) error {
	byteJSON, err := json.Marshal(v)
	if err != nil {
		return err
	}

	_, err = d.Cli.Put(context.TODO(), k, string(byteJSON))
	if err != nil {
		return err
	}
	return nil
}

// パブリックIPアドレスが一致するインスタンスを探す
func (d *Database) FindByPublicIPaddress(ipAddress string) (bool, error) {
	resp, err := d.GetEtcdByPrefix("vm")
	if err != nil {
		return false, err
	}
	for _, ev := range resp.Kvs {
		var vm VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			return false, nil /// 例外的にエラーを無視
		}
		if ipAddress == vm.PublicIp {
			return true, nil
		}
	}
	return false, nil
}

// プライベートIPアドレスが一致するインスンスを探す
func (d *Database) FindByPrivateIPaddress(ipAddress string) (bool, error) {
	resp, err := d.GetEtcdByPrefix("vm")
	if err != nil {
		return false, err
	}
	for _, ev := range resp.Kvs {
		var vm VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			return false, nil /// データが存在しない時には、どうするか？
		}
		if ipAddress == vm.PrivateIp {
			return true, nil
		}
	}
	return false, nil
}

// ホスト名からVMキーを探す
func (d *Database) FindByHostname(hostname string) (string, error) {
	resp, err := d.GetEtcdByPrefix("vm")
	if err != nil {
		return "", err
	}
	//var vm VirtualMachine
	for _, ev := range resp.Kvs {
		var vm VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			return "", err
		}
		if hostname == vm.Name {
			return vm.Key, err
		}
	}
	return "", errors.New("NotFound")
}

// ホスト名とクラスタ名でVMキーを取得する
func (d *Database) FindByHostAndClusteName(hostname string, clustername string) (string, error) {
	resp, err := d.GetEtcdByPrefix("vm")
	if err != nil {
		return "", err
	}

	for _, ev := range resp.Kvs {
		var vm VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			return "", err
		}
		if hostname == vm.Name && clustername == vm.ClusterName {
			return vm.Key, err
		}
	}
	return "", errors.New("NotFound")
}
