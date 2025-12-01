package db

import (
	"encoding/json"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
)

type JobEntry struct {
	Kind      string    // 要求種類  request,cancel
	Id        string    // UUID
	Status    int       // 実行状態
	Key       string    // etcdに登録したキー
	ReqTime   time.Time // ジョブの依頼時刻
	StartTime time.Time // ジョブの開始時刻
	FinTime   time.Time // ジョブの終了時刻
	MaxTime   time.Time // 上限のタスク実行時間
	Task      string    // ジョブの内容コマンド
	ExitCode  int       // ジョブ終了コード
}

const (
	JOB_PENDING = 0 // 0 実行待ち
	JOB_RUNNING = 1 // 1 実行中
	JOB_DELETED = 2 // 2 削除済み
	JOB_ERROR   = 3 // 3 エラー終了
	JOB_SUCCESS = 4 // 4 正常終了
)

// ジョブ登録
// ここでは登録だけにして、実行はしない。
func (d *Database) RegisterJob(task string) (string, error) {
	var job JobEntry
	job.Kind = "request"
	id, err := uuid.NewRandom()
	if err != nil {
		slog.Error("failed to generate uuid", "err", err)
		return "", err
	}
	job.Id = id.String()
	job.Key = JobPrefix + "/" + job.Id
	job.ReqTime = time.Now()
	job.Task = task
	if err := d.PutDataEtcd(job.Key, job); err != nil {
		slog.Error("PutDataEtcd() failed", "err", err, "key", job.Key)
		return "", err
	}
	return job.Id, nil
}

// ジョブ番号を指定してジョブをキャンセルする
func (d *Database) CancelJob(id string) error {
	key := JobPrefix + "/" + id
	if err := d.DelByKey(key); err != nil {
		slog.Error("CancelJob() failed", "err", err, "key", key)
		return err
	}
	return nil
}

// 古いものから順番にジョブのリストを取得する
func (d *Database) GetJobs() ([]JobEntry, error) {
	var jobs []JobEntry
	resp, err := d.GetEtcdByPrefix(JobPrefix)
	if err != nil {
		slog.Error("failed to getting prefix", "err", err)
		return nil, err
	}
	if resp.Count == 0 {
		return nil, nil
	}
	for _, ev := range resp.Kvs {
		var job JobEntry
		if err := json.Unmarshal([]byte(ev.Value), &job); err != nil {
			slog.Error("failed to Unmarchall", "err", err)
			return nil, err
		}
		jobs = append(jobs, job)
	}
	// ジョブ登録時間の順でソート
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].ReqTime.Unix() < jobs[j].ReqTime.Unix()
	})

	return jobs, nil
}

// 未処理ジョブの最も古いものを取得する
func (d *Database) FetchJob() (JobEntry,error) {
	jobs, err := d.GetJobs()
	if err != nil {
		return JobEntry{},err
	}
	if len(jobs) == 0 {
		return JobEntry{}, nil
	}
	
	return jobs[0], nil
}

// ジョブの実行（ここが適切か？)
