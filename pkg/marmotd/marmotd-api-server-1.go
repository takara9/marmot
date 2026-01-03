package marmotd

import (
	_ "embed"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

//go:embed version.txt
var Version string

// DEBUG Print
const DEBUG bool = true

type Marmot struct {
	NodeName string
	EtcdUrl  string
	Db       *db.Database
}

type Server struct {
	Lock sync.Mutex
	Ma   *Marmot
}

// Marmot インスタンスの生成、これにより関数コールが可能となる
// etcdUrl は、etcd サーバーの URL を指定する
// nodeName は、ハイパーバイザーのノード名を指定する
func NewMarmot(nodeName string, etcdUrl string) (*Marmot, error) {
	var m Marmot
	var err error
	m.Db, err = db.NewDatabase(etcdUrl)
	if err != nil {
		return nil, err
	}
	m.NodeName = nodeName
	m.EtcdUrl = etcdUrl
	return &m, nil
}

// Marmot インスタンスの終了
func (m *Marmot) Close() error {
	return m.Db.Close()
}

// marmotd サーバーの生成、REST API サーバーを起動する
// marmotdで定義された関数に対して、REST API 経由でアクセスできるようにする
func NewServer(node string, etcdurl string) *Server {
	marmotInstance, err := NewMarmot(node, etcdurl)
	if err != nil {
		slog.Error("Storage free space check", "err", err)
		os.Exit(1)
	}
	return &Server{
		Ma: marmotInstance,
	}
}

// サーバーの終了
func (s *Server) Close() error {
	return s.Ma.Db.Close()
}

// ＝＝＝＝＝＝＝＝＝＝＝＝＝＝　API 関数群  ＝＝＝＝＝＝＝＝＝＝＝＝＝＝
// 生存確認
func (s *Server) ReplyPing(ctx echo.Context) error {
	slog.Debug("===", "ReplyPing() is called", "===")
	s.Lock.Lock()
	defer s.Lock.Unlock()
	return ctx.JSON(http.StatusOK, api.ReplyMessage{Message: "ok"})
}

// バージョン取得
func (s *Server) GetVersion(ctx echo.Context) error {
	slog.Debug("===", "GetVersion() is called", "===")
	s.Lock.Lock()
	defer s.Lock.Unlock()
	var v api.Version
	v.ServerVersion = &Version
	return ctx.JSON(http.StatusOK, v)
}

// ハイパーバイザーのリスト
func (s *Server) ListHypervisors(ctx echo.Context, params api.ListHypervisorsParams) error {
	slog.Debug("===", "ListHypervisors() is called", "===")
	s.Lock.Lock()
	defer s.Lock.Unlock()

	// データベースから情報を取得
	d, err := db.NewDatabase(s.Ma.EtcdUrl)
	if err != nil {
		slog.Error("connect to database", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	_, err = d.CheckHypervisors(s.Ma.EtcdUrl, s.Ma.NodeName)
	if err != nil {
		slog.Error("Check if the hypervisor is up and running", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// ストレージ容量の更新 結果はDBへ反映
	if err := d.CheckHvVgAllByName(s.Ma.NodeName); err != nil {
		slog.Error("Update storage capacity", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	var hvs []api.Hypervisor
	if err := d.GetHypervisors(&hvs); err != nil {
		slog.Error("get hypervisor status", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, hvs)
}

// 仮想マシンのリスト
func (s *Server) ListVirtualMachines(ctx echo.Context) error {
	slog.Debug("===", "ListVirtualMachines() is called", "===")
	s.Lock.Lock()
	defer s.Lock.Unlock()

	d, err := db.NewDatabase(s.Ma.EtcdUrl)
	if err != nil {
		slog.Error("setup error at new database connection", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	var vms []api.VirtualMachine
	err = d.GetVmsStatuses(&vms)
	if err == db.ErrNotFound {
		slog.Debug("get vm status: no vm found", "err", err)
	} else if err != nil {
		slog.Error("get vm status", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	//vms2 := convVMinfoDBtoAPI(vms)
	return ctx.JSON(http.StatusOK, vms)
}

// 仮想マシンのクラスタを作成
func (s *Server) CreateCluster(ctx echo.Context) error {
	slog.Debug("===", "CreateCluster() is called", "===")
	s.Lock.Lock()
	defer s.Lock.Unlock()

	var cnf api.MarmotConfig
	err := ctx.Bind(&cnf)
	if err != nil {
		slog.Error("Creating cluster", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// データベースから情報を取得
	d, err := db.NewDatabase(s.Ma.EtcdUrl)
	if err != nil {
		slog.Error("connect to database", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// ハイパーバイザーの稼働チェック 結果はDBへ反映
	_, err = d.CheckHypervisors(s.Ma.EtcdUrl, s.Ma.NodeName)
	if err != nil {
		slog.Error("check hypervisor status", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if err := s.Ma.CreateClusterInternal(cnf); err != nil {
		slog.Error("create cluster", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusCreated, nil)
}

// 仮想マシンのクラスタを削除
func (s *Server) DestroyCluster(ctx echo.Context) error {
	slog.Debug("===", "DestroyCluster() is called", "===")
	s.Lock.Lock()
	defer s.Lock.Unlock()

	var cnf api.MarmotConfig
	err := ctx.Bind(&cnf)
	if err != nil {
		slog.Error("DestroyCluster()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if err := s.Ma.DestroyClusterInternal(cnf); err != nil {
		slog.Error("DestroyCluster()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, nil)
}

// 仮想マシンクラスタの開始（再スタート）
func (s *Server) StartCluster(ctx echo.Context) error {
	slog.Debug("===", "StartCluster() is called", "===")
	s.Lock.Lock()
	defer s.Lock.Unlock()

	var cnf api.MarmotConfig
	err := ctx.Bind(&cnf)
	if err != nil {
		slog.Error("StartCluster()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if err := s.Ma.StartClusterInternal(cnf); err != nil {
		slog.Error("StartCluster()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusCreated, nil)
}

// 仮想マシンクラスタの停止（一時停止）
func (s *Server) StopCluster(ctx echo.Context) error {
	slog.Debug("===", "StopCluster() is called", "===")
	s.Lock.Lock()
	defer s.Lock.Unlock()

	var cnf api.MarmotConfig
	err := ctx.Bind(&cnf)
	if err != nil {
		slog.Error("StopCluster()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if err := s.Ma.StopClusterInternal(cnf); err != nil {
		slog.Error("StopCluster()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusCreated, nil)
}

// 仮想マシンの生成
func (s *Server) CreateVirtualMachine(ctx echo.Context) error {
	slog.Debug("===", "CreateVirtualMachine() is called", "===")

	// ここをロックすると、テストが実施できない。実際にも動かないかも
	//s.Lock.Lock()
	//defer s.Lock.Unlock()

	var spec api.VmSpec
	err := ctx.Bind(&spec)
	if err != nil {
		slog.Error("CreateVirtualMachine()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	err = s.Ma.CreateVM(spec)
	if err != nil {
		slog.Error("CreateVirtualMachine()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusCreated, nil)
}

// 仮想マシンの削除
func (s *Server) DestroyVirtualMachine(ctx echo.Context) error {
	slog.Debug("===", "DestroyVirtualMachine() is called", "===")
	//s.Lock.Lock()
	//defer s.Lock.Unlock()

	var spec api.VmSpec
	err := ctx.Bind(&spec)
	if err != nil {
		slog.Error("DestroyVirtualMachine()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	slog.Debug("DestroyVirtualMachine()", "spec.Key===", spec.Key)
	if spec.Key == nil {
		slog.Debug("DestroyVirtualMachine()", "spec.Key is nil", 0)
	} else {
		slog.Debug("DestroyVirtualMachine()", "spec.Key", *spec.Key)
	}

	err = s.Ma.DestroyVM2(spec)
	if err != nil {
		slog.Error("DestroyVirtualMachine()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, nil)
}

func (s *Server) StopVirtualMachine(ctx echo.Context) error {
	slog.Debug("===", "StopVirtualMachine() is called", "===")
	//s.Lock.Lock()
	//defer s.Lock.Unlock()

	var spec api.VmSpec
	err := ctx.Bind(&spec)
	if err != nil {
		slog.Error("StopVirtualMachine()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(200, nil)
}

func (s *Server) StartVirtualMachine(ctx echo.Context) error {
	slog.Debug("===", "StopVirtualMachine() is called", "===")
	//s.Lock.Lock()
	//defer s.Lock.Unlock()

	var spec api.VmSpec
	err := ctx.Bind(&spec)
	if err != nil {
		slog.Error("StartVirtualMachine()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(200, nil)
}

func (s *Server) ShowHypervisorById(ctx echo.Context, hypervisorId string) error {
	slog.Debug("===", "ShowHypervisorById() is called", "===")
	//s.Lock.Lock()
	//defer s.Lock.Unlock()

	var hvs []api.Hypervisor
	err := s.Ma.Db.GetHypervisors(&hvs)
	if err != nil {
		slog.Error("ShowHypervisorById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if len(hvs) < 1 {
		slog.Error("ShowHypervisorById()", "id", hypervisorId)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: ""})
	}

	for _, v := range hvs {
		if hypervisorId == v.NodeName {
			return ctx.JSON(http.StatusOK, v)
		}
	}
	return ctx.JSON(http.StatusNotFound, api.ReplyMessage{Message: "Hypervisor " + hypervisorId + " not found"})
}
<<<<<<< HEAD:pkg/marmotd/marmotd-api-server-1.go
=======

// CreateVolume implements api.ServerInterface.
func (s *Server) CreateVolume(ctx echo.Context) error {
	slog.Debug("===", "CreateVolume() is called", "===")
	var volSpec api.Volume

	err := ctx.Bind(&volSpec)
	if err != nil {
		volSpecString, err2 := json.MarshalIndent(volSpec, "", "  ")
		slog.Error("CreateVolume()", "err", err, "volSpec", string(volSpecString), "err2", err2)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// キーだけでなく、スペック全体を返すようにする
	spec, err := s.Ma.CreateNewVolume(volSpec)
	if err != nil {
		slog.Error("CreateNewVolume()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("CreateVolume()", "volKey", *spec.Key)

	return ctx.JSON(http.StatusCreated, spec)
}

// DeleteVolumeById implements api.ServerInterface.
func (s *Server) DeleteVolumeById(ctx echo.Context, id string) error {
	slog.Debug("===", "DeleteVolumeById() is called", "===", "volumeId", id)

	//key := "/" + id
	err := s.Ma.RemoveVolume(id)
	if err != nil {
		slog.Error("RemoveVolume()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, api.ReplyMessage{Message: "Successfully deleted"})
}

// ListVolumes implements api.ServerInterface.
func (s *Server) ListVolumes(ctx echo.Context) error {
	slog.Debug("===", "ListVolumes() is called", "===")
	vols, err := s.Ma.GetDataVolumes()
	if err != nil {
		slog.Error("ListVolumes()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	vols2, err := s.Ma.GetOsVolumes()
	if err != nil {
		slog.Error("ListVolumes()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	vols = append(vols, vols2...)

	return ctx.JSON(http.StatusOK, vols)
}

// ShowVolumeById implements api.ServerInterface.
func (s *Server) ShowVolumeById(ctx echo.Context, volumeId string) error {
	slog.Debug("===", "ShowVolumeById() is called", "===", "volumeId", volumeId)
	vol, err := s.Ma.ShowVolumeById(volumeId)
	if err != nil {
		slog.Error("ShowVolumeById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("ShowVolumeById()", "vol", vol)

	return ctx.JSON(http.StatusOK, vol)
}

// UpdateVolumeById implements api.ServerInterface.
func (s *Server) UpdateVolumeById(ctx echo.Context, volumeId string) error {
	slog.Debug("===", "UpdateVolumeById() is called", "===", "volumeId", volumeId)
	var volSpec api.Volume
	if err := ctx.Bind(&volSpec); err != nil {
		slog.Error("UpdateVolumeById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	key := db.VolumePrefix + "/" + volumeId
	if _, err := s.Ma.UpdateVolumeById(volumeId, volSpec); err != nil {
		slog.Error("UpdateVolumeById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	volSpec.Key = &key
	volSpec.Id = volumeId

	return ctx.JSON(http.StatusOK, volSpec)
}

// サーバーのリストを取得、フィルターは、パラメータで指定するようにする
func (s *Server) GetServers(ctx echo.Context) error {
	slog.Debug("===", "GetServers() is called", "===")
	var serverSpec api.Server
	if err := ctx.Bind(&serverSpec); err != nil {
		slog.Error("GetServers()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	_, err := s.Ma.GetServers()
	if err != nil {
		slog.Error("GetServers()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return nil
}

// サーバーの作成
func (s *Server) CreateServer(ctx echo.Context) error {
	slog.Debug("===", "CreateServer() is called", "===")

	var serverSpec api.Server
	if err := ctx.Bind(&serverSpec); err != nil {
		slog.Error("CreateServer()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	if err := s.Ma.CreateServer(serverSpec); err != nil {
		slog.Error("CreateServer()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return nil
}

// サーバーの詳細を取得
func (s *Server) GetServerById(ctx echo.Context, id string) error {
	slog.Debug("===", "GetServerById() is called", "===")
	var serverSpec api.Server
	if err := ctx.Bind(&serverSpec); err != nil {
		slog.Error("GetServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if err := s.Ma.GetServerById(serverSpec); err != nil {
		slog.Error("GetServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return nil
}

// サーバーの削除
func (s *Server) DeleteServerById(ctx echo.Context, id string) error {
	slog.Debug("===", "DeleteServerById() is called", "===")
	var serverSpec api.Server
	if err := ctx.Bind(&serverSpec); err != nil {
		slog.Error("DeleteServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if err := s.Ma.DeleteServerById(serverSpec.Id); err != nil {
		slog.Error("DeleteServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return nil
}

// サーバーの更新
func (s *Server) UpdateServerById(ctx echo.Context, id string) error {
	slog.Debug("===", "UpdateServerById() is called", "===")
	var serverSpec api.Server
	if err := ctx.Bind(&serverSpec); err != nil {
		slog.Error("DeleteServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if err := s.Ma.UpdateServerById(serverSpec); err != nil {
		slog.Error("UpdateServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return nil
}
>>>>>>> origin/main:pkg/marmotd/marmotd.go
