package db

import (
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	. "github.com/takara9/marmot/pkg/types"
	etcd "go.etcd.io/etcd/client/v3"
)

// シリアル番号
func (d *Database) CreateSeq(key string, start uint64, step uint64) error {
	var seq VmSerial
	seq.Serial = start
	seq.Start = start
	seq.Step = step
	seq.Key = SeqPrefix + "/" + key
	err := d.PutJSON(seq.Key, seq)
	return err
}

// 内部関数
func (d *Database) getSeqRaw(key string) (uint64, error) {
	etcdKey := SeqPrefix + "/" + key
	resp, err := d.Cli.Get(d.Ctx, etcdKey, etcd.WithLimit(1))
	if err != nil {
		slog.Error("GetSeqByKind()", "err", err, "etcdKey", etcdKey)
		return 0, err
	}
	if resp.Count == 0 {
		slog.Error("GetSeqByKind() NotFound", "etcdKey", etcdKey)
		return 0, errors.New("NotFound")
	}
	expected := resp.Kvs[0].ModRevision

	var seq VmSerial
	if err = json.Unmarshal(resp.Kvs[0].Value, &seq); err != nil {
		return 0, err
	}

	// シリアル番号のインクリメント
	seqno := seq.Serial
	seq.Serial = seq.Serial + seq.Step

	// CASで更新
	err = d.PutJSONCAS(etcdKey, expected, &seq)
	if err != nil {
		slog.Error("PutJSONCAS() failed", "err", err, "etcdKey", etcdKey, "expected", expected)
		return 0, err
	}
	return seqno, nil
}

// シリアル番号の取得（CASロック）
func (d *Database) GetSeqByKind(key string) (uint64, error) {
	if len(key) == 0 {
		slog.Error("GetSeqByKind() key is empty")
		return 0, errors.New("key is empty")
	}

	lockKey := "/lock/seq/" + key
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return 0, err
	}
	defer d.UnlockKey(mutex)
	for retry := 0; retry < 5; retry++ {
		if seqno, err := d.getSeqRaw(key); err == nil {
			return seqno, nil
		} else {
			slog.Warn("GetSeqByKind() retrying", "retry", retry, "err", err)
			time.Sleep(100 * time.Millisecond) // 少し待ってからリトライ
		}
	}
	slog.Error("GetSeqByKind() exceeded retry limit", "key", key)
	return 0, errors.New("GetSeqByKind() exceeded retry limit")
}

func (d *Database) GetSeqs(seqs *[]VmSerial) error {
	resp, err := d.GetByPrefix(SeqPrefix + "/")
	if err != nil {
		return err
	}

	for _, ev := range resp.Kvs {
		var seq VmSerial
		if err := json.Unmarshal(ev.Value, &seq); err != nil {
			return err
		}
		*seqs = append(*seqs, seq)
	}

	return nil
}

func (d *Database) DelSeqByKey(key string) error {
	lockKey := "/lock/seq/" + key
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)
	return d.DeleteJSON(SeqPrefix + "/" + key)
}
