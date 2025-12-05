package db

import (
	"encoding/json"
	"errors"
	"log/slog"

	. "github.com/takara9/marmot/pkg/types"
)

// シリアル番号
func (d *Database) CreateSeq(key string, start uint64, step uint64) error {
	var seq VmSerial
	seq.Serial = start
	seq.Start = start
	seq.Step = step
	seq.Key = SeqPrefix+"/"+key
	err := d.PutDataEtcd(seq.Key, seq)
	return err
}

// シリアル番号の取得（ロックが必須）
func (d *Database) GetSeqByKind(key string) (uint64, error) {
	// 排他制御
	d.Lock.Lock()
	defer d.Lock.Unlock()

	// etcdキーを使ったシリアル番号の取得
	etcdKey := SeqPrefix + "/" + key
	resp, err := d.Cli.Get(d.Ctx, etcdKey)
	if err != nil {
		slog.Error("GetSeqByKind()", "err", err, "etcdKey", etcdKey)
		return 0, err
	}
	if resp.Count == 0 {
		slog.Error("GetSeqByKind() NotFound", "etcdKey", etcdKey)
		return 0, errors.New("NotFound")
	}

	var seq VmSerial
	if err = json.Unmarshal(resp.Kvs[0].Value, &seq); err != nil {
		return 0, err
	}

	// シリアル番号のインクリメントと保存
	seqno := seq.Serial
	seq.Serial = seq.Serial + seq.Step
	if err := d.PutDataEtcd(etcdKey, seq); err != nil {
		return 0, err
	}

	return seqno, nil
}

func (d *Database) GetSeqs(seqs *[]VmSerial) error {
	resp, err := d.GetEtcdByPrefix(SeqPrefix + "/")
	if err != nil {
		return err
	}
	if resp.Count == 0 {
		return errors.New("NotFound")
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
	return d.DelByKey(SeqPrefix + "/" + key)
}
