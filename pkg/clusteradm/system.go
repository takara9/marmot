// Package clusteradm は maadm cluster コマンドの実処理を提供する。
package clusteradm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	etcd "go.etcd.io/etcd/client/v3"
)

const (
	DefaultConfigPath = "/etc/marmot/marmotd.json"
	// etcd.service 本体は変更せず、drop-in で DAEMON_ARGS を上書きする
	DefaultDropInPath = "/etc/systemd/system/etcd.service.d/marmot-cluster.conf"

	EtcdUnit   = "etcd.service"
	MarmotUnit = "marmot.service"

	DefaultUnderlayInterface = "br0"
	etcdClientPort           = 2379
)

// ClusterStore はクラスタ情報の保存先（etcd）を抽象化する。
type ClusterStore interface {
	// クラスタが無効なら db.ErrNotFound を返す
	GetClusterInfo() (*api.ClusterInfo, error)
	PutClusterInfo(info api.ClusterInfo) error
	DeleteClusterInfo() error
	ListHostStatus() ([]api.HostStatus, error)
	DeleteHostStatus(nodeName string) error
	// 離脱ノードの follower ネットワークを削除する。残すと、再参加時のメンバー同期で削除日時が設定され、
	// 同名ネットワークへの削除伝播により head の mgmt 等が削除対象になる
	DeleteFollowerNetworks(nodeName string) error
	// 離脱ノードの follower ネットワークに対応する、ローカルの libvirt ネットワーク名を返す
	ListFollowerNetworkNames(nodeName string) ([]string, error)
	// スケジューラーが、ノードへ新しい VM を配置しないようにする
	CordonNode(nodeName string) error
	// VM やネットワークなど、クラスタへ引き継がれない管理対象のキーを返す
	ListManagedObjects() ([]string, error)
	// 指定ノードに割り当てられた VM を返す
	ListServersOnNode(nodeName string) ([]ServerRef, error)
	// ステータスを STOPPING にして、ノードのコントローラーにシャットダウンを依頼する
	StopServer(id string) error
	// 削除日時をセットして、ノードのコントローラーに削除を依頼する
	DeleteServer(id string) error
	// 指定したボリュームのうち、etcd にまだ残っているものの ID を返す
	ExistingVolumeIDs(ids []string) ([]string, error)
	Close() error
}

// ServerRef は、離脱するノード上の VM の識別情報
type ServerRef struct {
	ID         string
	Name       string
	StatusCode int
	// サーバーの削除と共に削除されるボリューム（ブート／非永続データ）の ID
	VolumeIDs []string
}

// Env は OS への副作用をまとめたもので、テストで差し替える。
type Env struct {
	ConfigPath string
	DropInPath string

	IsRoot        func() bool
	InterfaceIPv4 func(name string) (string, error)
	Systemctl     func(args ...string) error
	// etcd.service に設定された環境変数（systemctl show -p Environment の値）を返す
	EtcdEnvironment func() (string, error)
	UnitActive      func(unit string) bool
	HostID          func() (string, error)
	EtcdHealth      func(ctx context.Context, url string) error
	OpenStore       func(url string) (ClusterStore, error)
	NewClusterID    func() string
	Now             func() time.Time

	// 外部コマンドを実行して標準出力を返す（ovs-vsctl など）
	Run func(name string, args ...string) (string, error)

	WaitTimeout     time.Duration
	ShutdownTimeout time.Duration
	DeleteTimeout   time.Duration
	PollInterval    time.Duration
}

func DefaultEnv() *Env {
	return &Env{
		ConfigPath:      DefaultConfigPath,
		DropInPath:      DefaultDropInPath,
		IsRoot:          func() bool { return os.Geteuid() == 0 },
		InterfaceIPv4:   interfaceIPv4,
		Systemctl:       systemctl,
		EtcdEnvironment: etcdEnvironment,
		UnitActive:      unitActive,
		HostID:          hostID,
		EtcdHealth:      etcdHealth,
		OpenStore:       openDBStore,
		NewClusterID:    uuid.NewString,
		Now:             time.Now,
		Run:             runCommand,
		WaitTimeout:     30 * time.Second,
		ShutdownTimeout: 120 * time.Second,
		DeleteTimeout:   300 * time.Second,
		PollInterval:    time.Second,
	}
}

func interfaceIPv4(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", fmt.Errorf("interface %q not found: %w", name, err)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("failed to get addresses of %q: %w", name, err)
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok {
			if ip4 := ipNet.IP.To4(); ip4 != nil {
				return ip4.String(), nil
			}
		}
	}
	return "", fmt.Errorf("interface %q has no IPv4 address", name)
}

func systemctl(args ...string) error {
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func etcdEnvironment() (string, error) {
	out, err := exec.Command("systemctl", "show", EtcdUnit, "-p", "Environment", "--value").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func runCommand(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("%s %s failed: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func unitActive(unit string) bool {
	return exec.Command("systemctl", "is-active", "--quiet", unit).Run() == nil
}

// marmotd が HOSTID として登録する値と同じ形式（8桁の16進数）で返す
func hostID() (string, error) {
	out, err := exec.Command("hostid").Output()
	if err != nil {
		return "", fmt.Errorf("hostid command failed: %w", err)
	}
	text := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(string(out))), "0x")
	v, err := strconv.ParseUint(text, 16, 32)
	if err != nil || v == 0 {
		return "", fmt.Errorf("hostid command returned an unexpected value: %q", strings.TrimSpace(string(out)))
	}
	return fmt.Sprintf("%08x", v), nil
}

func etcdHealth(ctx context.Context, url string) error {
	cli, err := etcd.New(etcd.Config{Endpoints: []string{url}, DialTimeout: 3 * time.Second})
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err = cli.Status(ctx, url)
	return err
}

type dbStore struct {
	d *db.Database
}

func openDBStore(url string) (ClusterStore, error) {
	d, err := db.NewDatabase(url)
	if err != nil {
		return nil, err
	}
	return &dbStore{d: d}, nil
}

func (s *dbStore) GetClusterInfo() (*api.ClusterInfo, error) { return s.d.GetClusterInfo() }
func (s *dbStore) PutClusterInfo(info api.ClusterInfo) error { return s.d.PutClusterInfo(info) }
func (s *dbStore) Close() error                              { return s.d.Close() }

func (s *dbStore) DeleteClusterInfo() error {
	ctx, cancel := context.WithTimeout(s.d.Ctx, 5*time.Second)
	defer cancel()
	_, err := s.d.Cli.Delete(ctx, db.ClusterInfoKey)
	return err
}

func (s *dbStore) ListHostStatus() ([]api.HostStatus, error) { return s.d.GetAllHostStatus() }

func (s *dbStore) CordonNode(nodeName string) error {
	return s.d.SetHostUnschedulable(nodeName, true)
}

func (s *dbStore) DeleteHostStatus(nodeName string) error {
	ctx, cancel := context.WithTimeout(s.d.Ctx, 5*time.Second)
	defer cancel()
	_, err := s.d.Cli.Delete(ctx, db.HostStatusPrefix+"/"+nodeName)
	return err
}

func (s *dbStore) ListFollowerNetworkNames(nodeName string) ([]string, error) {
	networks, err := s.d.GetVirtualNetworks()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, n := range networks {
		if n.Metadata.NodeName == nil || strings.TrimSpace(*n.Metadata.NodeName) != nodeName {
			continue
		}
		if n.Metadata.Labels == nil || db.GetNetworkSyncRole(*n.Metadata.Labels) != "follower" {
			continue
		}
		if name := strings.TrimSpace(n.Metadata.Name); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func (s *dbStore) DeleteFollowerNetworks(nodeName string) error {
	networks, err := s.d.GetVirtualNetworks()
	if err != nil {
		return err
	}
	for _, n := range networks {
		if n.Metadata.NodeName == nil || strings.TrimSpace(*n.Metadata.NodeName) != nodeName {
			continue
		}
		if n.Metadata.Labels == nil || db.GetNetworkSyncRole(*n.Metadata.Labels) != "follower" {
			continue
		}
		ctx, cancel := context.WithTimeout(s.d.Ctx, 5*time.Second)
		_, err := s.d.Cli.Delete(ctx, db.NetworkPrefix+"/"+api.VirtualNetworkID(n))
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *dbStore) ListServersOnNode(nodeName string) ([]ServerRef, error) {
	servers, err := s.d.GetServers()
	if err != nil {
		return nil, err
	}
	var refs []ServerRef
	for _, sv := range servers {
		if sv.Metadata.NodeName == nil || *sv.Metadata.NodeName != nodeName {
			continue
		}
		ref := ServerRef{ID: api.ServerID(sv), Name: sv.Metadata.Name}
		if sv.Status != nil {
			ref.StatusCode = sv.Status.StatusCode
		}
		if sv.Spec.BootVolume != nil && api.VolumeID(*sv.Spec.BootVolume) != "" {
			ref.VolumeIDs = append(ref.VolumeIDs, api.VolumeID(*sv.Spec.BootVolume))
		}
		if sv.Spec.Storage != nil {
			for _, vol := range *sv.Spec.Storage {
				if (vol.Spec.Persistent != nil && *vol.Spec.Persistent) || api.VolumeID(vol) == "" {
					continue
				}
				ref.VolumeIDs = append(ref.VolumeIDs, api.VolumeID(vol))
			}
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (s *dbStore) StopServer(id string) error {
	return s.d.UpdateServerStatus(id, db.SERVER_STOPPING, "")
}

func (s *dbStore) DeleteServer(id string) error { return s.d.SetDeleteTimestamp(id) }

func (s *dbStore) ExistingVolumeIDs(ids []string) ([]string, error) {
	var existing []string
	for _, id := range ids {
		ctx, cancel := context.WithTimeout(s.d.Ctx, 5*time.Second)
		resp, err := s.d.Cli.Get(ctx, db.VolumePrefix+"/"+id, etcd.WithCountOnly())
		cancel()
		if err != nil {
			return nil, err
		}
		if resp.Count > 0 {
			existing = append(existing, id)
		}
	}
	return existing, nil
}

// marmotd が自動作成する予約ネットワーク。管理対象としては数えない
// host-bridge は libvirt 上の実体が etcd へ自動インポートされるため、同様に除外する
var autoManagedNetworkNames = map[string]bool{
	"mgmt":        true,
	"host-bridge": true,
}

func (s *dbStore) ListManagedObjects() ([]string, error) {
	prefixes := []string{
		db.ServerPrefix, db.VolumePrefix, db.NetworkPrefix, db.GatewayPrefix, db.VpnGatewayPrefix,
		db.LoadBalancerPrefix, db.NetworkLoadBalancerPrefix, db.KubernetesEnginePrefix,
	}
	var found []string
	for _, prefix := range prefixes {
		ctx, cancel := context.WithTimeout(s.d.Ctx, 5*time.Second)
		resp, err := s.d.Cli.Get(ctx, prefix+"/", etcd.WithPrefix())
		cancel()
		if err != nil {
			return nil, err
		}
		for _, kv := range resp.Kvs {
			key := string(kv.Key)
			if prefix == db.NetworkPrefix {
				if strings.Contains(key, "/ip_network/") {
					continue
				}
				var vnet api.VirtualNetwork
				if json.Unmarshal(kv.Value, &vnet) == nil && autoManagedNetworkNames[vnet.Metadata.Name] {
					continue
				}
			}
			found = append(found, key)
		}
	}
	return found, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, db.ErrNotFound)
}
