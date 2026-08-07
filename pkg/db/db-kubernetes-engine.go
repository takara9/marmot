package db

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

const (
	KUBERNETES_ENGINE_PENDING  = 0
	KUBERNETES_ENGINE_DELETING = 1
)

var KubernetesEngineStatus = map[int]string{
	KUBERNETES_ENGINE_PENDING:  "PENDING",
	KUBERNETES_ENGINE_DELETING: "DELETING",
}

// CreateKubernetesEngine は KubernetesEngine を etcd に登録する。
// この時点ではコントローラーによる実クラスタ構築は行わず、PENDING状態で書き込むのみ。
func (d *Database) CreateKubernetesEngine(spec api.KubernetesEngine) (api.KubernetesEngine, error) {
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

func (d *Database) DeleteKubernetesEngineById(id string) error {
	lockKey := "/lock/kubernetes-engine/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)
	return d.DeleteJSON(KubernetesEnginePrefix + "/" + id)
}
