package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

// ジョブ機能のインスタンス
type Job struct {
	Database   *Database
	jobLogPath string // ジョブログの保存パス
}

const (
	JOB_PENDING   = 0 // 実行待ち
	JOB_RUNNING   = 1 // 実行中
	JOB_CANCELED  = 2 // 削除済み
	JOB_FAILED    = 3 // 失敗終了
	JOB_SUCCEEDED = 4 // 正常終了
	JOB_PATH      = "/var/lib/marmot/jobs"
)

var JobStatus = map[int]string{
	0: "PENDING",
	1: "RUNNING",
	2: "CANCELED",
	3: "FAILED",
	4: "SUCCEEDED",
}

// 更新ロックを入れること

// ジョブコントローラの生成
func NewJobController(url, jobLogPath string) (*Job, error) {
	// データベース接続の生成
	d, err := NewDatabase(url)
	if err != nil {
		slog.Error("failed to create database", "err", err)
		return nil, err
	}

	// ジョブログの保存ディレクトリの作成
	jobLogDefaultPath := JOB_PATH
	if len(jobLogPath) == 0 {
		err := os.Mkdir(jobLogDefaultPath, 0750)
		if err != nil && !os.IsExist(err) {
			slog.Error("failed to create job log directory", "err", err)
			return nil, err
		}
	}

	return &Job{
		Database:   d,
		jobLogPath: jobLogDefaultPath,
	}, nil
}

// 新しいジョブの登録
func (d *Job) EntryJob(name string, cmd ...string) (string, error) {
	var job api.Job
	var spec api.JobSpec
	var metadata api.Metadata
	var status api.Status
	var command []string
	job.Metadata = &metadata
	job.Spec = &spec
	job.Spec.Command = &command
	job.Status = &status

	// 一意のジョブIDを生成
	for {
		tid, err := uuid.NewRandom()
		if err != nil {
			slog.Error("failed to generate uuid", "err", err)
			return "", err
		}
		x := tid.String()
		id := x[:5]
		key := JobPrefix + "/" + id
		_, err = d.Database.GetJSON(key, &job)
		if err == ErrNotFound {
			job.Id = id
			break
		} else if err != nil {
			slog.Error("failed to get job entry", "err", err, "key", key)
			return "", err
		}
	}

	slog.Info("EntryJob()", "jobId", job.Id, "name", name, "cmd", cmd)
	x := time.Now()
	job.Spec.RequestTime = util.TimePtr(x)
	job.Metadata.Name = util.StringPtr(name)
	for _, c := range cmd {
		*job.Spec.Command = append(*job.Spec.Command, c)
	}
	job.Status.StatusCode = JOB_PENDING
	job.Status.Status = util.StringPtr(JobStatus[job.Status.StatusCode])
	job.Status.CreationTimeStamp = util.TimePtr(time.Now())
	job.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())

	// ジョブのIDの重複は発生しないので、排他制御しない
	Key := JobPrefix + "/" + job.Id
	if err := d.Database.PutJSON(Key, job); err != nil {
		slog.Error("failed to put job entry", "err", err, "key", Key)
		return "", err
	}
	return job.Id, nil
}

func (d *Job) GetJob(id string) (api.Job, error) {
	var job api.Job
	key := JobPrefix + "/" + id
	if _, err := d.Database.GetJSON(key, &job); err != nil {
		slog.Error("failed to get job entry", "err", err, "key", key)
		return api.Job{}, err
	}
	return job, nil
}

// ジョブ番号を指定してジョブをキャンセルする
func (d *Job) CancelJob(id string) error {
	// ロックする
	lockName := JobPrefix + "/" + id
	m, err := d.Database.LockKey(lockName)
	if err != nil {
		slog.Error("failed to acquire lock for canceling job", "err", err, "lockName", lockName)
		return fmt.Errorf("failed to acquire lock for canceling job %s: %w", id, err)
	}
	defer d.Database.UnlockKey(m)

	// キーでデータを取得
	job, err := d.GetJob(id)
	if err != nil {
		slog.Error("CancelJob() failed to get job entry", "err", err, "id", id)
		return err
	}

	// 状態をキャンセルにして
	job.Status.StatusCode = JOB_CANCELED
	job.Status.Status = util.StringPtr(JobStatus[job.Status.StatusCode])
	job.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())

	// データを保存する
	key := JobPrefix + "/" + id
	if err := d.Database.PutJSON(key, job); err != nil {
		slog.Error("CancelJob() failed to update job entry", "err", err, "key", key)
		return err
	}
	return nil
}

// 古いものから順番にジョブのリストを取得する
func (d *Job) GetJobs(jobStatus int) ([]api.Job, error) {
	var jobs []api.Job
	resp, err := d.Database.GetByPrefix(JobPrefix)
	if err != nil {
		slog.Error("failed to getting prefix", "err", err)
		return nil, err
	}
	if resp.Count == 0 {
		return nil, nil
	}
	for _, ev := range resp.Kvs {
		var job api.Job
		if err := json.Unmarshal([]byte(ev.Value), &job); err != nil {
			slog.Error("failed to Unmarchall", "err", err)
			return nil, err
		}
		// 未処理ジョブのみ抽出
		if job.Status.StatusCode == jobStatus {
			jobs = append(jobs, job)
		}
	}
	// ジョブ登録時間の順でソート
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Spec.RequestTime.Unix() < jobs[j].Spec.RequestTime.Unix()
	})

	return jobs, nil
}

// 古いものから順番にジョブのリストを取得する
func (d *Job) GetAllJobs() ([]api.Job, error) {
	var jobs []api.Job
	resp, err := d.Database.GetByPrefix(JobPrefix)
	if err != nil {
		slog.Error("failed to getting prefix", "err", err)
		return nil, err
	}
	if resp.Count == 0 {
		return nil, nil
	}
	for _, ev := range resp.Kvs {
		var job api.Job
		if err := json.Unmarshal([]byte(ev.Value), &job); err != nil {
			slog.Error("failed to Unmarchall", "err", err)
			return nil, err
		}
		jobs = append(jobs, job)
	}
	// ジョブ登録時間の順でソート
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Spec.RequestTime.Unix() < jobs[j].Spec.RequestTime.Unix()
	})

	return jobs, nil
}

// 未処理ジョブの最も古いものを取得する
func (d *Job) FetchJob() (api.Job, error) {
	jobs, err := d.GetJobs(JOB_PENDING)
	if err != nil {
		return api.Job{}, err
	}
	if len(jobs) == 0 {
		return api.Job{}, nil
	}

	return jobs[0], nil
}

// ジョブの実行
// Goルーチン内で、ジョブを起こして、標準出力、標準エラーをキャプチャして、ジョブログに保存する。
// ジョブの終了コードも保存する。
func (d *Job) RunJob(job api.Job) error {
	// ジョブログの作成
	jobLogFile := fmt.Sprintf("%s/%s.log", d.jobLogPath, job.Id)
	file, err := os.Create(jobLogFile)
	if err != nil {
		slog.Error("failed to create job log file", "err", err, "file", jobLogFile)
	}
	defer file.Close()

	fmt.Fprintf(file, "Job Name: [%v] ID [%v] at %v\n", *job.Metadata.Name, job.Id, *job.Spec.RequestTime)

	// ジョブ状態の更新 （ここだけ別関数にして排他制御を入れるのが良いかも）
	key := JobPrefix + "/" + job.Id
	job.Status.StatusCode = JOB_RUNNING
	job.Status.Status = util.StringPtr(JobStatus[job.Status.StatusCode])
	job.Spec.StartTime = util.TimePtr(time.Now())
	if err := d.Database.PutJSON(key, job); err != nil {
		slog.Error("failed to put job entry", "err", err, "key", key)
		return err
	}

	// ジョブの実行
	fmt.Println("===", (*job.Spec.Command)[0], (*job.Spec.Command)[1:], "===")
	cmd := exec.Command((*job.Spec.Command)[0], (*job.Spec.Command)[1:]...)
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
		slog.Error("failed to start command", "err", err, "cmd", *job.Spec.Command)
		fmt.Fprintln(file, "Failed to start command:", err)
	}

	fmt.Fprintln(file, "Command:", *job.Spec.Command)
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
		slog.Error("command execution failed", "err", err, "cmd", *job.Spec.Command)
		fmt.Fprintln(file, "Command execution failed:", err)
	}
	fmt.Fprintln(file, "Job Finished at", *job.Spec.StartTime, "with exit code=", cmd.ProcessState.ExitCode())

	// ジョブ状態の更新
	job.Spec.FinishTime = util.TimePtr(time.Now())
	job.Spec.ExitCode = util.IntPtrInt(cmd.ProcessState.ExitCode())
	if *job.Spec.ExitCode == 0 {
		job.Status.StatusCode = JOB_SUCCEEDED
		job.Status.Status = util.StringPtr(JobStatus[job.Status.StatusCode])
	} else {
		job.Status.StatusCode = JOB_FAILED
		job.Status.Status = util.StringPtr(JobStatus[job.Status.StatusCode])
	}

	slog.Info("***PUT ETCD", "key", key, "job-id", job.Id, "status", JobStatus[job.Status.StatusCode], "exit-code", *job.Spec.ExitCode)

	if err := d.Database.PutJSON(key, job); err != nil {
		slog.Error("failed to write Job data", "err", err, "key", key)
		return err
	}

	return nil
}

// ジョブとジョブログのクリーンナップ
func (d *Job) CleanupJob(t int) error {
	// 指定された秒数以上のIDをetcd と /var/lib/mamort/jobsから削除する
	jobs, err := d.GetAllJobs()
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.Spec.FinishTime != nil && time.Since(*job.Spec.FinishTime) > time.Duration(t)*time.Second {
			// ジョブの削除
			key := JobPrefix + "/" + job.Id
			if err := d.Database.DeleteJSON(key); err != nil {
				slog.Error("failed to delete job entry", "err", err, "key", key)
				return err
			}
			// ジョブログの削除
			jobLogFile := fmt.Sprintf("%s/%s.log", d.jobLogPath, job.Id)
			if err := os.Remove(jobLogFile); err != nil {
				slog.Error("failed to delete job log file", "err", err, "file", jobLogFile)
				return err
			}
			slog.Info("Cleaned up job and log file", "jobId", job.Id, "logFile", jobLogFile)
		}
	}
	return nil

}
