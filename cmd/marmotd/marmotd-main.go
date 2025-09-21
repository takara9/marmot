package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmot"
	"github.com/takara9/marmot/pkg/util"
)

type Server struct {
	Lock sync.Mutex
	mx   *marmot.Marmot
}

func NewServer(node string, etcdurl string) *Server {
	marmotInstance, err := marmot.NewMarmot(node, etcdurl)
	if err != nil {
		slog.Error("Storage free space check", "err", err)
		os.Exit(1)
	}
	return &Server{
		mx: marmotInstance,
	}
}

func main() {
	node := flag.String("node", "hv1", "Hypervisor node name")
	etcd := flag.String("etcd", "http://127.0.0.1:3379", "etcd url")
	flag.Parse()
	fmt.Println("node = ", *node)
	fmt.Println("etcd = ", *etcd)

	// Setup slog
	opts := &slog.HandlerOptions{
		AddSource: true,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	slog.SetDefault(logger)
	e := echo.New()

	Server := NewServer(*node, *etcd)

	api.RegisterHandlersWithBaseURL(e, Server, "/api/v1")
	// And we serve HTTP until the world ends.
	fmt.Println(e.Start("0.0.0.0:8750"))
}

// 生存確認
func (s *Server) ReplyPing(ctx echo.Context) error {
	s.Lock.Lock()
	defer s.Lock.Unlock()

	return ctx.JSON(http.StatusOK, api.ReplyMessage{Message: "ok"})
}

// バージョン取得
func (s *Server) GetVersion(ctx echo.Context) error {
	s.Lock.Lock()
	defer s.Lock.Unlock()

	return ctx.JSON(http.StatusOK, api.Version{Version: "0.0.1"})
}

// ハイパーバイザーのリスト
func (s *Server) ListHypervisors(ctx echo.Context, params api.ListHypervisorsParams) error {
	s.Lock.Lock()
	defer s.Lock.Unlock()

	_, err := util.CheckHypervisors(s.mx.EtcdUrl, s.mx.NodeName)
	if err != nil {
		slog.Error("Check if the hypervisor is up and running", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// ストレージ容量の更新 結果はDBへ反映
	err = util.CheckHvVgAll(s.mx.EtcdUrl, s.mx.NodeName)
	if err != nil {
		slog.Error("Update storage capacity", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// データベースから情報を取得
	d, err := db.NewDatabase(s.mx.EtcdUrl)
	if err != nil {
		slog.Error("connect to database", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	var hvs []db.Hypervisor
	err = d.GetHvsStatus(&hvs)
	if err != nil {
		slog.Error("get hypervisor status", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, hvs)
}

// 仮想マシンのリスト（テストできていない）
func (s *Server) ListVirtualMachines(ctx echo.Context) error {
	fmt.Println("========== ListVirtualMachines ===========")
	s.Lock.Lock()
	defer s.Lock.Unlock()

	d, err := db.NewDatabase(s.mx.EtcdUrl)
	if err != nil {
		slog.Error("get list virtual machines", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	var vms []db.VirtualMachine
	err = d.GetVmsStatus(&vms)
	if err != nil {
		slog.Error("get status of virtual machines", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	vms2 := convVMinfoDBtoAPI(vms)
	return ctx.JSON(http.StatusOK, vms2)
}

// 仮想マシンのクラスタを作成
func (s *Server) CreateCluster(ctx echo.Context) error {
	fmt.Println("============== RECV Server: Create Cluster =================")
	s.Lock.Lock()
	defer s.Lock.Unlock()

	var cnf config.MarmotConfig
	err := ctx.Bind(&cnf)
	if err != nil {
		slog.Error("Creating cluster", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// ハイパーバイザーの稼働チェック 結果はDBへ反映
	_, err = util.CheckHypervisors(s.mx.EtcdUrl, s.mx.NodeName)
	if err != nil {
		slog.Error("check hypervisor status", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	newCnf := marmot.ConvConfClusterOld2New(cnf)
	if err := s.mx.CreateClusterInternal(newCnf); err != nil {
		slog.Error("create cluster", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusCreated, nil)
}

// 仮想マシンのクラスタを削除
func (s *Server) DestroyCluster(ctx echo.Context) error {
	fmt.Println("============== RECV Server: Destroy Cluster =================")
	s.Lock.Lock()
	defer s.Lock.Unlock()

	var cnf config.MarmotConfig
	err := ctx.Bind(&cnf)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	newCnf := marmot.ConvConfClusterOld2New(cnf)
	if err := s.mx.DestroyClusterInternal(newCnf); err != nil {
		slog.Error("create cluster", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, nil)
}

// 仮想マシンクラスタの開始（再スタート）
func (s *Server) StartCluster(ctx echo.Context) error {
	s.Lock.Lock()
	defer s.Lock.Unlock()
	fmt.Println("=========== StartCluster ============")

	var cnf config.MarmotConfig
	err := ctx.Bind(&cnf)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	newCnf := marmot.ConvConfClusterOld2New(cnf)
	if err := s.mx.DestroyClusterInternal(newCnf); err != nil {
		slog.Error("create cluster", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusCreated, nil)
}

// 仮想マシンクラスタの停止（一時停止）
func (s *Server) StopCluster(ctx echo.Context) error {
	s.Lock.Lock()
	defer s.Lock.Unlock()
	fmt.Println("=========== StopCluster ============")

	var cnf config.MarmotConfig
	err := ctx.Bind(&cnf)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	newCnf := marmot.ConvConfClusterOld2New(cnf)
	if err := s.mx.StopClusterInternal(newCnf); err != nil {
		slog.Error("create cluster", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusCreated, nil)
}

// 仮想マシンの生成
func (s *Server) CreateVirtualMachine(ctx echo.Context) error {
	// ここをロックすると、テストが実施できない。実際にも動かないかも
	//s.Lock.Lock()
	//defer s.Lock.Unlock()
	var spec api.VmSpec
	err := ctx.Bind(&spec)
	fmt.Println("=========== CreateVirtualMachine ============", err)
	fmt.Println("Spec=", spec)
	fmt.Println("Spec Name =", *spec.Name)
	fmt.Println("Spec Cpu =", *spec.Cpu)
	fmt.Println("Spec Memory =", *spec.Memory)
	err = s.mx.CreateVM2(spec)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusCreated, nil)
}

// 仮想マシンの削除
func (s *Server) DestroyVirtualMachine(ctx echo.Context) error {
	//s.Lock.Lock()
	//defer s.Lock.Unlock()
	var spec api.VmSpec
	err := ctx.Bind(&spec)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	err = s.mx.DestroyVM2(spec)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, nil)
}

func (s *Server) StopVirtualMachine(ctx echo.Context) error {
	//s.Lock.Lock()
	//defer s.Lock.Unlock()
	var spec api.VmSpec
	err := ctx.Bind(&spec)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	fmt.Println("=========== StopVirtualMachine ============", err)
	fmt.Println("Spec=", spec)
	fmt.Println("Spec Key =", *spec.Key)

	return ctx.JSON(200, nil)
}

func (s *Server) StartVirtualMachine(ctx echo.Context) error {
	//s.Lock.Lock()
	//defer s.Lock.Unlock()
	var spec api.VmSpec
	err := ctx.Bind(&spec)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	fmt.Println("=========== StartVirtualMachine ============", err)
	fmt.Println("Spec=", spec)
	fmt.Println("Spec Key =", *spec.Key)

	return ctx.JSON(200, nil)
}

func (s *Server) ShowHypervisorById(ctx echo.Context, hypervisorId string) error {
	s.Lock.Lock()
	defer s.Lock.Unlock()

	var hvs []db.Hypervisor
	err := s.mx.Db.GetHvsStatus(&hvs)
	if err != nil {
		slog.Error("get hypervisor status", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if len(hvs) < 1 {
		slog.Error("No such hypervisor", "id", hypervisorId)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	for _, v := range hvs {
		if hypervisorId == v.Nodename {
			nhv := convHVinfoDBtoAPI(v)
			return ctx.JSON(http.StatusOK, nhv)
		}
	}
	return ctx.JSON(http.StatusNotFound, api.ReplyMessage{Message: "Hypervisor " + hypervisorId + " not found"})
}
