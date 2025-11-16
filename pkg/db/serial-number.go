package db

import (
	"encoding/json"
	"errors"
	"fmt"

	. "github.com/takara9/marmot/pkg/types"
)

// シリアル番号
func (d *Database) CreateSeq(key string, start uint64, step uint64) error {
	etcd_key := fmt.Sprintf("SEQNO_%v", key)
	var seq VmSerial
	seq.Serial = start
	seq.Start = start
	seq.Step = step
	seq.Key = key
	err := d.PutDataEtcd(etcd_key, seq)
	return err
}

// シリアル番号の取得
func (d *Database) GetSeq(key string) (uint64, error) {
	var seq VmSerial

	etcdKey := fmt.Sprintf("SEQNO_%v", key)
	resp, err := d.Cli.Get(d.Ctx, etcdKey)
	if err != nil {
		return 0, err
	}

	err = json.Unmarshal(resp.Kvs[0].Value, &seq)
	if err != nil {
		return 0, err
	}

	seqno := seq.Serial
	seq.Serial = seq.Serial + seq.Step

	err = d.PutDataEtcd(etcdKey, seq)
	if err != nil {
		return 0, err
	}
	return seqno, nil
}

func (d *Database) GetSeqs(seqs *[]VmSerial) error {
	resp, err := d.GetEtcdByPrefix("SEQNO_")
	if err != nil {
		return err
	}
	if resp.Count == 0 {
		return errors.New("NotFound")
	}

	for _, ev := range resp.Kvs {
		var seq VmSerial
		err = json.Unmarshal(ev.Value, &seq)
		if err != nil {
			return err
		}
		*seqs = append(*seqs, seq)
	}

	return nil
}

func (d *Database) DelSeq(key string) error {
	etcdKey := fmt.Sprintf("SEQNO_%v", key)
	err := d.DelByKey(etcdKey)
	return err
}
