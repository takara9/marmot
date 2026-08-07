package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

const (
	KUBERNETES_ENGINE_PENDING      = 0
	KUBERNETES_ENGINE_PROVISIONING = 1
	KUBERNETES_ENGINE_RUNNING      = 2
	KUBERNETES_ENGINE_DELETING     = 3
	KUBERNETES_ENGINE_FAILED       = 4
)

var KubernetesEngineStatus = map[int]string{
	KUBERNETES_ENGINE_PENDING:      "PENDING",
	KUBERNETES_ENGINE_PROVISIONING: "PROVISIONING",
	KUBERNETES_ENGINE_RUNNING:      "RUNNING",
	KUBERNETES_ENGINE_DELETING:     "DELETING",
	KUBERNETES_ENGINE_FAILED:       "FAILED",
}

// CreateKubernetesEngine は KubernetesEngine を etcd に登録する。
// この時点ではコントローラーによる実クラスタ構築は行わず、PENDING状態で書き込むのみ。
func (d *Database) CreateKubernetesEngine(spec api.KubernetesEngine) (api.KubernetesEngine, error) {
	mutex, err := d.LockKey("/lock/kubernetes-engine/create")
	if err != nil {
		return api.KubernetesEngine{}, err
	}
	defer d.UnlockKey(mutex)

	current, err := d.GetKubernetesEngines()
	if err != nil && err != ErrNotFound {
		return api.KubernetesEngine{}, err
	}
	name := strings.TrimSpace(spec.Metadata.Name)
	for _, k := range current {
		if strings.TrimSpace(k.Metadata.Name) == name {
			return api.KubernetesEngine{}, fmt.Errorf("kubernetes engine with name %q already exists", name)
		}
	}

	kubernetesEngine, err := util.DeepCopy(spec)
	if err != nil {
		return api.KubernetesEngine{}, err
	}

	var key string
	for {
		kubernetesEngine.Metadata.Uuid = util.StringPtr(uuid.New().String())
		id := (*kubernetesEngine.Metadata.Uuid)[:5]
		api.SetKubernetesEngineID(&kubernetesEngine, id)
		key = KubernetesEnginePrefix + "/" + id

		var existing api.KubernetesEngine
		_, getErr := d.GetJSON(key, &existing)
		if getErr == ErrNotFound {
			break
		}
		if getErr != nil {
			return api.KubernetesEngine{}, getErr
		}
	}

	now := time.Now()
	kubernetesEngine.Status = &api.Status{
		StatusCode:          KUBERNETES_ENGINE_PENDING,
		Status:              util.StringPtr(KubernetesEngineStatus[KUBERNETES_ENGINE_PENDING]),
		CreationTimeStamp:   util.TimePtr(now),
		LastUpdateTimeStamp: util.TimePtr(now),
	}
	if err := d.PutJSON(key, kubernetesEngine); err != nil {
		return api.KubernetesEngine{}, err
	}
	return kubernetesEngine, nil
}

func (d *Database) GetKubernetesEngines() ([]api.KubernetesEngine, error) {
	result := make([]api.KubernetesEngine, 0)
	resp, err := d.GetByPrefix(KubernetesEnginePrefix)
	if err == ErrNotFound {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	for _, kv := range resp.Kvs {
		var rec api.KubernetesEngine
		if err := json.Unmarshal(kv.Value, &rec); err != nil {
			slog.Error("GetKubernetesEngines() unmarshal failed", "err", err)
			continue
		}
		result = append(result, rec)
	}
	return result, nil
}

func (d *Database) GetKubernetesEngineById(id string) (api.KubernetesEngine, error) {
	key := KubernetesEnginePrefix + "/" + id
	var rec api.KubernetesEngine
	_, err := d.GetJSON(key, &rec)
	if err != nil {
		return api.KubernetesEngine{}, err
	}
	return rec, nil
}

// DeleteKubernetesEngineById は etcd から即座にレコードを削除する（ハードデリート）。
// API経由の削除要求は SetDeleteTimestampKubernetesEngine で DELETING 状態に遷移させ、
// コントローラーが猶予期間経過後にこの関数を呼び出して実削除する想定。
func (d *Database) DeleteKubernetesEngineById(id string) error {
	lockKey := "/lock/kubernetes-engine/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)
	return d.DeleteJSON(KubernetesEnginePrefix + "/" + id)
}

// SetDeleteTimestampKubernetesEngine は DeletionTimeStamp を設定して DELETING 状態に遷移させる。
func (d *Database) SetDeleteTimestampKubernetesEngine(id string) error {
	for {
		rec, err := d.GetKubernetesEngineById(id)
		if err != nil {
			return err
		}
		if rec.Status == nil {
			rec.Status = &api.Status{}
		}
		rec.Status.StatusCode = KUBERNETES_ENGINE_DELETING
		rec.Status.Status = util.StringPtr(KubernetesEngineStatus[KUBERNETES_ENGINE_DELETING])
		rec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
		if rec.Status.DeletionTimeStamp == nil {
			rec.Status.DeletionTimeStamp = util.TimePtr(time.Now())
		}
		rec.Status.Message = nil

		err = d.putKubernetesEngineById(rec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

// UpdateKubernetesEngineStatusWithMessage は状態コードとメッセージを更新する。
// コントローラーがライフサイクルの状態遷移（Pending→Provisioning→Running等）を
// 進める際に使用する。
func (d *Database) UpdateKubernetesEngineStatusWithMessage(id string, status int, message string) error {
	for {
		rec, err := d.GetKubernetesEngineById(id)
		if err != nil {
			return err
		}
		if rec.Status == nil {
			rec.Status = &api.Status{}
		}
		statusChanged := rec.Status.StatusCode != status
		rec.Status.StatusCode = status
		rec.Status.Status = util.StringPtr(KubernetesEngineStatus[status])
		rec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
		trimmed := strings.TrimSpace(message)
		if statusChanged {
			rec.Status.Message = nil
		}
		if trimmed != "" {
			rec.Status.Message = util.StringPtr(trimmed)
		}

		err = d.putKubernetesEngineById(rec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

func (d *Database) putKubernetesEngineById(rec api.KubernetesEngine) error {
	id := api.KubernetesEngineID(rec)
	if strings.TrimSpace(id) == "" {
		return ErrNotFound
	}

	lockKey := "/lock/kubernetes-engine/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	key := KubernetesEnginePrefix + "/" + id
	var current api.KubernetesEngine
	resp, err := d.GetJSON(key, &current)
	if err != nil {
		return err
	}
	api.SetKubernetesEngineID(&rec, id)
	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, &rec)
}
