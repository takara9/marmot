package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/google/uuid"
)

// ジョブ機能のインスタンス
type Job struct {
	Database    *Database
	jobs        map[string]JobEntry // etcdに保存する
	jobsContext map[string]exec.Cmd // 実行中のジョブコンテキスト
	jobLogPath  string              // ジョブログの保存パス
}

// 個別のジョブ
type JobEntry struct {
	Kind      string    // 要求種類  request,cancel
	Id        string    // UUID
	Status    int       // 実行状態
	Key       string    // etcdに登録したキー
	ReqTime   time.Time // ジョブの依頼時刻
	StartTime time.Time // ジョブの開始時刻
	FinTime   time.Time // ジョブの終了時刻
	MaxTime   time.Time // 上限のタスク実行時間
	JobName   string    // ジョブの名前
	Cmd       []string  // ジョブのコマンド
	ExitCode  int       // ジョブ終了コード

}

const (
	JOB_PENDING = 1 // 実行待ち
	JOB_RUNNING = 2 // 実行中
	JOB_DELETED = 3 // 削除済み
	JOB_ERROR   = 4 // エラー終了
	JOB_SUCCESS = 5 // 正常終了
)

// ジョブコントローラの生成
func NewJobController(url, jobLogPath string) (*Job, error) {
	// データベース接続の生成
	d, err := NewDatabase(url)
	if err != nil {
		slog.Error("failed to create database", "err", err)
		return nil, err
	}

	// ジョブログの保存ディレクトリの作成
	jobLogDefaultPath := "/tmp/joblogs"
	if len(jobLogPath) == 0 {
		err := os.Mkdir(jobLogDefaultPath, 0750)
		if err != nil && !os.IsExist(err) {
			slog.Error("failed to create job log directory", "err", err)
			return nil, err
		}
	}

	return &Job{
		Database:    d,
		jobs:        make(map[string]JobEntry),
		jobsContext: make(map[string]exec.Cmd),
		jobLogPath:  jobLogDefaultPath,
	}, nil
}

// 新しいジョブの登録
func (d *Job) EntryJob(name string, cmd ...string) (string, error) {
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
	job.JobName = name
	job.Cmd = cmd
	job.Status = JOB_PENDING

	mutex, err := d.Database.LockKey(job.Key)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", job.Key)
		return "", err
	}
	defer d.Database.UnlockKey(mutex)

	if err := d.Database.PutJSON(job.Key, job); err != nil {
		slog.Error("failed to write", "err", err, "key", job.Key)
		return "", err
	}

	return job.Id, nil
}

// ジョブ番号を指定してジョブをキャンセルする
func (d *Job) CancelJob(id string) error {
	key := JobPrefix + "/" + id
	if err := d.Database.DeleteJSON(key); err != nil {
		slog.Error("failed to delete job", "err", err, "key", key)
		return err
	}
	return nil
}

// 古いものから順番にジョブのリストを取得する
func (d *Job) GetJobs(jobStatus int) ([]JobEntry, error) {
	var jobs []JobEntry
	resp, err := d.Database.GetByPrefix(JobPrefix)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return jobs, nil
		}
		slog.Error("failed to getting prefix", "err", err)
		return jobs, err
	}

	for _, ev := range resp.Kvs {
		var job JobEntry
		if err := json.Unmarshal([]byte(ev.Value), &job); err != nil {
			slog.Error("failed to Unmarchall", "err", err)
			return nil, err
		}
		// 未処理ジョブのみ抽出
		if jobStatus == 0 {
			jobs = append(jobs, job)
		} else if job.Status == jobStatus {
			jobs = append(jobs, job)
		}
	}
	// ジョブ登録時間の順でソート
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].ReqTime.Unix() < jobs[j].ReqTime.Unix()
	})

	return jobs, nil
}

// 未処理ジョブの最も古いものを取得する
func (d *Job) FetchJob() (JobEntry, error) {
	jobs, err := d.GetJobs(JOB_PENDING)
	if err != nil {
		return JobEntry{}, err
	}
	if len(jobs) == 0 {
		return JobEntry{}, nil
	}

	return jobs[0], nil
}

// ジョブの実行（ここが適切か？)
// Goルーチン内で、ジョブを起こして、標準出力、標準エラーをキャプチャして、ジョブログに保存する。
// ジョブの終了コードも保存する。
func (d *Job) RunJob(job JobEntry) error {
	// ジョブログの作成
	jobLogFile := fmt.Sprintf("%s/%s.log", d.jobLogPath, job.Id)
	file, err := os.Create(jobLogFile)
	if err != nil {
		slog.Error("failed to create job log file", "err", err, "file", jobLogFile)
	}
	defer file.Close()
	fmt.Fprintf(file, "Job Name: [%v] ID [%v] at %v\n", job.JobName, job.Id, job.StartTime.String())

	// ジョブ状態の更新 （ここだけ別関数にして排他制御を入れるのが良いかも）
	job.Status = JOB_RUNNING
	job.StartTime = time.Now()
	key := JobPrefix + "/" + job.Id

	mutex, err := d.Database.LockKey(key)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", key)
		return err
	}

	if err := d.Database.PutJSON(key, job); err != nil {
		slog.Error("failed to write", "err", err, "key", job.Key)
		return err
	}
	
	// ロックのリリース
	d.Database.UnlockKey(mutex)

	// ジョブの実行
	fmt.Println("===", job.Cmd[0], job.Cmd[1:], "===")
	cmd := exec.Command(job.Cmd[0], job.Cmd[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error("failed to get stdout pipe", "err", err)
		fmt.Fprintln(file, "Failed to get stdout pipe:", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		slog.Error("failed to get stderr pipe", "err", err)
		fmt.Fprintln(file, "Failed to get stderr pipe:", err)
	}
	if err := cmd.Start(); err != nil {
		slog.Error("failed to start command", "err", err, "cmd", job.Cmd)
		fmt.Fprintln(file, "Failed to start command:", err)
	}

	fmt.Fprintln(file, "Command:", job.Cmd)
	fmt.Fprintln(file, "Stdout:")

	buf := make([]byte, 1024)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			fmt.Fprint(file, "  ", string(buf[:n]))
		} else {
			break
		}
		if err != nil {
			slog.Error("failed to read stdout", "err", err)
			fmt.Fprintln(file, "Failed to read stdout:", err)
			break
		}
	}

	fmt.Fprintln(file, "Stderr:")
	for {
		n, err := stderr.Read(buf)
		if n > 0 {
			fmt.Fprint(file, "  ", string(buf[:n]))
		} else {
			break
		}
		if err != nil {
			slog.Error("failed to read stderr", "err", err)
			fmt.Fprintln(file, "Failed to read stderr:", err)
			break
		}
	}

	// ジョブの終了待ち
	if err := cmd.Wait(); err != nil {
		slog.Error("command execution failed", "err", err, "cmd", job.Cmd)
		fmt.Fprintln(file, "Command execution failed:", err)
	}
	fmt.Fprintln(file, "Job Finished at", job.FinTime.String(), "with exit code=", cmd.ProcessState.ExitCode())

	// ジョブ状態の更新
	job.FinTime = time.Now()
	job.ExitCode = cmd.ProcessState.ExitCode()
	if job.ExitCode == 0 {
		job.Status = JOB_SUCCESS
	} else {
		job.Status = JOB_ERROR
	}
	
	mutex, err = d.Database.LockKey(key)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", key)
		return err
	}
	defer d.Database.UnlockKey(mutex)

	if err := d.Database.PutJSON(key, job); err != nil {
		slog.Error("failed to write", "err", err, "key", key)
		return err
	}

	return nil
}
