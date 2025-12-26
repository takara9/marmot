package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/util"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// ハイパーバイザーの設定
func (d *Database) SetHypervisors(v config.Hypervisor_yaml) error {
	var hv api.Hypervisor
	hv.NodeName = v.Name
	hv.Port = util.Int64PtrInt32(v.Port)
	hvKey := HvPrefix + "/" + v.Name
	hv.Key = &hvKey
	hv.IpAddr = &v.IpAddr
	hv.Cpu = int32(v.Cpu) // 必須項目のためポインタではない
	hv.FreeCpu = util.Int64PtrInt32(v.Cpu)
	hv.Memory = util.Int64PtrConvMB(v.Ram)
	hv.FreeMemory = util.Int64PtrConvMB(v.Ram)
	hv.Status = util.Int64PtrInt32(2) // 暫定

	var stgpool []api.StoragePool
	for _, val := range v.Storage {
		var sp api.StoragePool
		sp.VolGroup = &val.VolGroup
		sp.Type = &val.Type
		stgpool = append(stgpool, sp)
	}
	hv.StgPool = &stgpool

	if err := d.PutDataEtcd(hvKey, hv); err != nil {
		slog.Error("PutDataEtcd()", "err", err)
		return err
	}

	return nil
}

func (d *Database) NewHypervisor(node string, hv api.Hypervisor) error {
	hv.NodeName = node
	etcdKey := HvPrefix + "/" + node
	hv.Key = &etcdKey
	hv.Status = util.Int64PtrInt32(2) // 暫定

	if err := d.PutDataEtcd(etcdKey, hv); err != nil {
		slog.Error("PutDataEtcd()", "err", err)
		return err
	}

	return nil
}

// Keyに一致したHVデータの取り出し
func (d *Database) GetHypervisorByName(hbNode string) (api.Hypervisor, error) {
	var hv api.Hypervisor
	ctx, _ := context.WithTimeout(d.Ctx, 5*time.Second)
	resp, err := d.Cli.Get(ctx, HvPrefix+"/"+hbNode)
	if err != nil {
		slog.Error("GetHypervisorByName()", "err", err)
		return api.Hypervisor{}, err
	}

	if resp.Count == 0 {
		return hv, errors.New("not found")
	}

	if err = json.Unmarshal([]byte(resp.Kvs[0].Value), &hv); err != nil {
		slog.Error("Unmarshal()", "err", err)
		return api.Hypervisor{}, err
	}

	return hv, err
}

// Keyに一致したHVを削除
func (d *Database) DeleteHypervisorByName(name string) error {
	if err := d.DeleteDataByKey(HvPrefix + "/" + name); err != nil {
		return err
	}
	return nil
}

// ハイパーバイザーのデータを取得
func (d *Database) GetHypervisors(hvs *[]api.Hypervisor) error {
	resp, err := d.GetDataByPrefix(HvPrefix)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}

	for _, ev := range resp.Kvs {
		var hv api.Hypervisor
		err = json.Unmarshal([]byte(ev.Value), &hv)
		if err != nil {
			return err
		}
		*hvs = append(*hvs, hv)
	}
	return nil
}

func (d *Database) CheckHvVgAllByName(nodeName string) error {
	slog.Debug("CheckHvVgAllByName()", "nodeName", nodeName)

	mutex := concurrency.NewMutex(d.Session, "/lock/hypervisor/"+nodeName)
	if err := mutex.Lock(d.Ctx); err != nil {
		if errors.Is(err, rpctypes.ErrLeaseNotFound) {
			slog.Debug("lease not found, ignoring")
		} else {
			slog.Error("failed to acquire lock", "err", err.Error())
			return fmt.Errorf("failed to acquire lock: %w", err)
		}
	}
	defer func() {
		if err := mutex.Unlock(d.Ctx); err != nil {
			slog.Error("failed to release lock", "err", err.Error())
		}
	}()

	hv, err := d.GetHypervisorByName(nodeName)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	for i := 0; i < len(*hv.StgPool); i++ {
		total_sz, free_sz, err := lvm.CheckVG(*(*hv.StgPool)[i].VolGroup)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}
		(*hv.StgPool)[i].FreeCap = util.IntPtrInt64(int(free_sz / 1024 / 1024 / 1024))
		(*hv.StgPool)[i].VgCap = util.IntPtrInt64(int(total_sz / 1024 / 1024 / 1024))
	}

	if err := d.PutDataEtcd(HvPrefix+"/"+nodeName, hv); err != nil {
		slog.Error("", "err", err)
		return err
	}
	return nil
}

func (d *Database) CheckHvVG2ByName(nodeName string, vg string) error {
	slog.Debug("CheckHvVG2ByName()", "nodeName", nodeName, "vg", vg)

	mutex := concurrency.NewMutex(d.Session, "/lock/hypervisor/"+nodeName)
	if err := mutex.Lock(d.Ctx); err != nil {
		if errors.Is(err, rpctypes.ErrLeaseNotFound) {
			slog.Debug("lease not found, ignoring")
		} else {
			slog.Error("failed to acquire lock", "err", err.Error())
			return fmt.Errorf("failed to acquire lock: %w", err)
		}
	}
	defer func() {
		if err := mutex.Unlock(d.Ctx); err != nil {
			slog.Error("failed to release lock", "err", err.Error())
		}
	}()

	// LVMへのアクセス
	total_sz, free_sz, err := lvm.CheckVG(vg)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	hv, err := d.GetHypervisorByName(nodeName)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// 一致するVGにデータをセット
	for i := 0; i < len(*hv.StgPool); i++ {
		if *(*hv.StgPool)[i].VolGroup == vg {
			(*hv.StgPool)[i].FreeCap = util.IntPtrInt64(int(free_sz / 1024 / 1024 / 1024))
			(*hv.StgPool)[i].VgCap = util.IntPtrInt64(int(total_sz / 1024 / 1024 / 1024))
		}
	}

	// DBへ書き込み
	err = d.PutDataEtcd(HvPrefix+"/"+nodeName, hv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	return nil
}

// ハイパーバイザーをREST-APIでアクセスして疎通を確認、DBへ反映させる
func (d *Database) CheckHypervisors(dbUrl string, nodeName string) ([]api.Hypervisor, error) {
	slog.Debug("CheckHypervisors()", "dbUrl", dbUrl, "node", nodeName)

	var hvs []api.Hypervisor
	mutex := concurrency.NewMutex(d.Session, "/lock/hypervisor/"+nodeName)
	if err := mutex.Lock(d.Ctx); err != nil {
		if errors.Is(err, rpctypes.ErrLeaseNotFound) {
			slog.Debug("lease not found, ignoring")
		} else {
			slog.Error("failed to acquire lock", "err", err.Error())
			return hvs, fmt.Errorf("failed to acquire lock: %w", err)
		}
	}
	defer func() {
		if err := mutex.Unlock(d.Ctx); err != nil {
			slog.Error("failed to release lock", "err", err.Error())
		}
	}()

	if err := d.GetHypervisors(&hvs); err != nil {
		slog.Error("failed to get hypervisors", "err", err)
		return hvs, err
	}

	// 自ノードを含むハイパーバイザーの死活チェック、DBへ反映
	for _, val := range hvs {
		// ハイパーバイザーの状態をDBへ書き込み
		err := d.PutDataEtcd(*val.Key, val)
		if err != nil {
			slog.Error("failed to put hypervisor data", "err", err)
		}
	}
	return hvs, nil
}
