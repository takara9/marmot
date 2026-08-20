package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

// ProvisionKubernetesEngineEtcd はクラスタ専用etcdの一連のプロビジョニング
// (ポート採番→etcdバイナリ取得→systemdユニット作成・起動)を行い、採番した
// クライアントポート・ピアポートを返す。既にポートが採番済みの場合は再採番せず
// そのポートを再利用する(冪等)。
func ProvisionKubernetesEngineEtcd(database *db.Database, mkeConf *marmotd.MKEConfig, ownEtcdURL string, ke api.KubernetesEngine) (clientPort, peerPort int, err error) {
	clusterName := strings.TrimSpace(ke.Metadata.Name)
	if clusterName == "" {
		return 0, 0, fmt.Errorf("kubernetes engine metadata.name is empty")
	}
	networkNamespace, _, _, err := KubernetesEngineControlPlaneNetworkNames(clusterName)
	if err != nil {
		return 0, 0, err
	}

	if ke.Status != nil && ke.Status.EtcdClientPort != nil && ke.Status.EtcdPeerPort != nil {
		clientPort, peerPort = *ke.Status.EtcdClientPort, *ke.Status.EtcdPeerPort
	} else {
		engines, getErr := database.GetKubernetesEngines()
		if getErr != nil {
			return 0, 0, getErr
		}
		clientPort, peerPort, err = AllocateKubernetesEngineEtcdPorts(engines, ownEtcdURL, DefaultEtcdPortRangeStart, DefaultEtcdPortRangeEnd)
		if err != nil {
			return 0, 0, err
		}
		id := api.KubernetesEngineID(ke)
		if err = database.UpdateKubernetesEngineEtcdPorts(id, clientPort, peerPort); err != nil {
			return 0, 0, err
		}
	}

	binPath, err := EnsureEtcdBinary(DefaultEtcdBinaryCacheDir, mkeConf.EtcdVersion)
	if err != nil {
		return 0, 0, fmt.Errorf("EnsureEtcdBinary() failed: %w", err)
	}

	cfg := KubernetesEngineEtcdUnitConfig{
		ClusterName:      clusterName,
		EtcdBinaryPath:   binPath,
		DataDir:          filepath.Join(DefaultEtcdDataDir, clusterName),
		NetworkNamespace: networkNamespace,
		ClientPort:       clientPort,
		PeerPort:         peerPort,
	}
	if err := CreateKubernetesEngineEtcdUnit(cfg); err != nil {
		return 0, 0, err
	}
	return clientPort, peerPort, nil
}

// DeprovisionKubernetesEngineEtcd はクラスタ専用etcdのsystemdユニットを停止・削除し、データディレクトリも削除する。
// 同名クラスタが再作成された際に、古いetcdデータ(Nodeオブジェクト等)が再利用されるのを防ぐため。
func DeprovisionKubernetesEngineEtcd(clusterName string) error {
	if err := DeleteKubernetesEngineEtcdUnit(clusterName); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(DefaultEtcdDataDir, clusterName)); err != nil {
		return fmt.Errorf("failed to remove etcd data dir: %w", err)
	}
	return nil
}
