package db

import (
	"encoding/json"
	"errors"
	"log/slog"

	. "github.com/takara9/marmot/pkg/types"
)

// シリアル番号
func (d *Database) CreateSeq(seqId string, start uint64, step uint64) error {
	var seq VmSerial
	key := SeqPrefix + "/" + seqId
	seq.Serial = start
	seq.Start = start
	seq.Step = step
	seq.Key = key

	lockKey := "/lock/seq/" + seqId
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockKey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	return d.PutJSON(key, seq)
}

// シリアル番号の取得（ロックが必須）
func (d *Database) GetSeqByKind(seqId string) (uint64, error) {
	lockkey := "/lock/seq/" + seqId
	mutex, err := d.LockKey(lockkey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockkey)
		return 0, err
	}
	defer d.UnlockKey(mutex)

	// etcdキーを使ったシリアル番号の取得
	key := SeqPrefix + "/" + seqId
	var seq VmSerial
	if _, err := d.GetJSON(key, &seq); err != nil {
		slog.Error("failed to get sequence number", "err", err, "key", key)
		return 0, err
	}
	
	// シリアル番号のインクリメントと保存
	seqno := seq.Serial
	seq.Serial = seq.Serial + seq.Step

	if err := d.PutJSON(key, seq); err != nil {
		return 0, err
	}

	return seqno, nil
}

func (d *Database) GetSeqs(seqs *[]VmSerial) error {
	key := SeqPrefix + "/"	
	resp, err := d.GetByPrefix(key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
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

func (d *Database) DelSeqByKey(seqId string) error {
	lockkey := "/lock/seq/" + seqId
	mutex, err := d.LockKey(lockkey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockkey)
		return err
	}
	defer d.UnlockKey(mutex)

	key := SeqPrefix + "/" + seqId
	return d.DeleteJSON(key)
}
