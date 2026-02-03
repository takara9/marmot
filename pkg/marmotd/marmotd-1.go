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

