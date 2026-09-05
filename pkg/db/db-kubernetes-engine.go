package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
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
	KUBERNETES_ENGINE_UPGRADING    = 5
	KUBERNETES_ENGINE_SCALING_IN   = 6
	KUBERNETES_ENGINE_SCALING_OUT  = 7

	// KubernetesEngine が専用作成するノード間通信ネットワークの所有者ラベル
	KubernetesEngineNetworkLabelOwner          = "kubernetesEngineId"
	KubernetesEngineNetworkLabelManagedBy      = "managedBy"
	KubernetesEngineNetworkLabelManagedByValue = "kubernetes-engine-controller"

	// KubernetesEngineCloudProviderLabelApiKeyID は、cloud-controller-manager(mke-node-controller)
	// がmarmotd APIへアクセスする際に使用するAPIKeyのID(admin ユーザー配下)を保持する
	// Metadata.Labelsのキー。クラスタ削除時にこのラベルからAPIKeyを特定して失効させる。
	KubernetesEngineCloudProviderLabelApiKeyID = "kubernetesEngineCloudProviderApiKeyID"
)

var KubernetesEngineStatus = map[int]string{
	KUBERNETES_ENGINE_PENDING:      "PENDING",
	KUBERNETES_ENGINE_PROVISIONING: "PROVISIONING",
	KUBERNETES_ENGINE_RUNNING:      "RUNNING",
	KUBERNETES_ENGINE_DELETING:     "DELETING",
	KUBERNETES_ENGINE_FAILED:       "FAILED",
	KUBERNETES_ENGINE_UPGRADING:    "UPGRADING",
	KUBERNETES_ENGINE_SCALING_IN:   "SCALING_IN",
	KUBERNETES_ENGINE_SCALING_OUT:  "SCALING_OUT",
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

// UpdateKubernetesEngineEtcdPorts はクラスタ専用etcdのクライアントポート・ピアポートを
// Statusに記録する。KubernetesEngineコントローラーがクラスタ作成時に採番したポートを
// 保存し、他クラスタとの衝突判定や再起動時の再利用に使用する。
func (d *Database) UpdateKubernetesEngineEtcdPorts(id string, clientPort, peerPort int) error {
	for {
		rec, err := d.GetKubernetesEngineById(id)
		if err != nil {
			return err
		}
		if rec.Status == nil {
			rec.Status = &api.Status{}
		}
		rec.Status.EtcdClientPort = util.IntPtrInt(clientPort)
		rec.Status.EtcdPeerPort = util.IntPtrInt(peerPort)
		rec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())

		err = d.putKubernetesEngineById(rec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

// UpdateKubernetesEngineControlPlaneStatus はクラスタ専用コントロールプレーンの
// 接続情報と解決済みKubernetesバージョンをStatusに記録する。
func (d *Database) UpdateKubernetesEngineControlPlaneStatus(id, ipAddress, hostIPAddress string, apiServerPort int, resolvedVersion string) error {
	for {
		rec, err := d.GetKubernetesEngineById(id)
		if err != nil {
			return err
		}
		if rec.Status == nil {
			rec.Status = &api.Status{}
		}
		rec.Status.ApiServerPort = util.IntPtrInt(apiServerPort)
		rec.Status.ControlPlaneIpAddress = util.StringPtr(ipAddress)
		rec.Status.ControlPlaneHostIpAddress = util.StringPtr(hostIPAddress)
		rec.Status.ResolvedKubernetesVersion = util.StringPtr(resolvedVersion)
		rec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())

		err = d.putKubernetesEngineById(rec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

// UpdateKubernetesEngineCloudProviderApiKeyID は、cloud-controller-manager(mke-node-controller)
// がmarmotd APIへアクセスするために発行したAPIKeyのIDをMetadata.Labelsに記録する。
// クラスタ削除時にこのラベルからAPIKeyを特定して失効させる。
func (d *Database) UpdateKubernetesEngineCloudProviderApiKeyID(id, keyID string) error {
	for {
		rec, err := d.GetKubernetesEngineById(id)
		if err != nil {
			return err
		}
		labels := map[string]interface{}{}
		if rec.Metadata.Labels != nil {
			for k, v := range *rec.Metadata.Labels {
				labels[k] = v
			}
		}
		labels[KubernetesEngineCloudProviderLabelApiKeyID] = keyID
		rec.Metadata.Labels = &labels

		err = d.putKubernetesEngineById(rec)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

// UpdateKubernetesEngineSpec は spec.nodes と spec.version のみを更新する。
// nodeSpec(cpu/memory/network)は起動済みノードに反映できないため、変更があれば拒否する。
func (d *Database) UpdateKubernetesEngineSpec(id string, desired api.KubernetesEngineSpec) error {
	for {
		err := d.updateKubernetesEngineSpec(id, desired)
		if err == ErrUpdateConflict {
			continue
		}
		return err
	}
}

func (d *Database) updateKubernetesEngineSpec(id string, desired api.KubernetesEngineSpec) error {
	if desired.Nodes < 1 {
		return fmt.Errorf("spec.nodes must be greater than zero")
	}
	if strings.TrimSpace(desired.Version) == "" {
		return fmt.Errorf("spec.version is required")
	}

	lockKey := "/lock/kubernetes-engine/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	key := KubernetesEnginePrefix + "/" + id
	var rec api.KubernetesEngine
	resp, err := d.GetJSON(key, &rec)
	if err != nil {
		return err
	}
	expected := resp.Kvs[0].ModRevision

	if desired.NodeSpec != nil && !reflect.DeepEqual(desired.NodeSpec, rec.Spec.NodeSpec) {
		return fmt.Errorf("spec.nodeSpec cannot be changed after creation")
	}

	cmp, err := compareKubernetesVersions(desired.Version, rec.Spec.Version)
	if err != nil {
		return err
	}
	if cmp < 0 {
		return fmt.Errorf("spec.version downgrade is not supported (current: %s, desired: %s)", rec.Spec.Version, desired.Version)
	}
	if cmp > 0 {
		if err := validateKubernetesEngineVersionUpgrade(rec.Spec.Version, desired.Version); err != nil {
			return err
		}
	}

	rec.Spec.Nodes = desired.Nodes
	rec.Spec.Version = desired.Version

	return d.PutJSONCAS(key, expected, &rec)
}

// validateKubernetesEngineVersionUpgrade は、Kubernetesは1マイナーバージョンずつの
// アップグレードが原則であることから、メジャーバージョン変更や2つ以上のマイナーバージョンの
// 飛び越しを拒否する(ダウングレードは呼び出し元で既に拒否済みのため、ここでは扱わない)。
func validateKubernetesEngineVersionUpgrade(current, desired string) error {
	currentVersion, err := parseKubernetesVersion(current)
	if err != nil {
		return err
	}
	desiredVersion, err := parseKubernetesVersion(desired)
	if err != nil {
		return err
	}
	if desiredVersion[0] != currentVersion[0] {
		return fmt.Errorf("spec.version major version upgrade is not supported in a single step (current: %s, desired: %s)", current, desired)
	}
	if desiredVersion[1]-currentVersion[1] > 1 {
		return fmt.Errorf("spec.version upgrade must not skip more than one minor version (current: %s, desired: %s)", current, desired)
	}
	return nil
}

// compareKubernetesVersions は "major.minor" または "major.minor.patch" 形式のバージョン文字列を
// 数値として比較する。a<b なら負、a==b なら0、a>b なら正の値を返す。
func compareKubernetesVersions(a, b string) (int, error) {
	av, err := parseKubernetesVersion(a)
	if err != nil {
		return 0, err
	}
	bv, err := parseKubernetesVersion(b)
	if err != nil {
		return 0, err
	}
	for i := 0; i < len(av); i++ {
		if av[i] != bv[i] {
			return av[i] - bv[i], nil
		}
	}
	return 0, nil
}

// parseKubernetesVersion は "1.30" や "1.30.2" のようなバージョン文字列を
// 数値の配列 [major, minor, patch] に変換する(patch省略時は0扱い)。
func parseKubernetesVersion(v string) ([3]int, error) {
	var result [3]int
	raw := strings.TrimSpace(v)
	clean := strings.TrimPrefix(raw, "v")
	parts := strings.Split(clean, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return result, fmt.Errorf("invalid version format: %q", raw)
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return result, fmt.Errorf("invalid version format: %q", raw)
		}
		result[i] = n
	}
	return result, nil
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
