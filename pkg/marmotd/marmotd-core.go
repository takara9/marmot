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
	"github.com/takara9/marmot/pkg/virt"
)

//go:embed version.txt
var Version string

// DEBUG Print
const DEBUG bool = true

type Marmot struct {
	NodeName string
	EtcdUrl  string
	Db       *db.Database
	Virt     *virt.LibVirtEp
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
		slog.Error("Failed to initialize database", "err", err)
		return nil, err
	}
	m.NodeName = nodeName
	m.EtcdUrl = etcdUrl
	m.Virt, err = virt.NewLibVirtEp("qemu:///system")
	if err != nil {
		slog.Error("Failed to initialize libvirt endpoint", "err", err)
		return nil, err
	}
	return &m, nil
}

// Marmot インスタンスの終了
func (m *Marmot) Close() error {
	if m.Virt != nil {
		m.Virt.Close()
	}
	if m.Db != nil {
		if err := m.Db.Close(); err != nil {
			slog.Error("Failed to close database", "err", err)
			return err
		}
	}
	return nil
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
