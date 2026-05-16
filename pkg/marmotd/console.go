package marmotd

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/virt"
)

// ApiConsoleServerById upgrades the HTTP connection and relays it to screen on the server host.
func (s *Server) ApiConsoleServerById(ctx echo.Context) error {
	id := strings.TrimSpace(ctx.Param("id"))
	if id == "" {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "server id is required"})
	}

	server, err := s.Ma.GetServerManage(id)
	if err != nil {
		slog.Error("ApiConsoleServerById() failed to get server", "id", id, "err", err)
		if err == db.ErrNotFound {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	consolePath := ""
	if server.Status != nil && server.Status.Console != nil {
		consolePath = strings.TrimSpace(*server.Status.Console)
	}
	if consolePath == "" {
		consolePath = strings.TrimSpace(resolveConsolePathFallback(server))
	}
	if consolePath == "" {
		return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "console path is not available"})
	}

	hijacker, ok := ctx.Response().Writer.(http.Hijacker)
	if !ok {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: "response writer does not support hijacking"})
	}

	conn, _, err := hijacker.Hijack()
	if err != nil {
		slog.Error("ApiConsoleServerById() hijack failed", "id", id, "err", err)
		return err
	}

	if _, err := io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/plain; charset=utf-8\r\nConnection: close\r\n\r\n"); err != nil {
		conn.Close()
		return err
	}

	if err := relayConsole(conn, consolePath); err != nil {
		slog.Error("ApiConsoleServerById() relay failed", "id", id, "consolePath", consolePath, "err", err)
	}
	return nil
}

func resolveConsolePathFallback(server api.Server) string {
	if server.Metadata.InstanceName == nil || strings.TrimSpace(*server.Metadata.InstanceName) == "" {
		return ""
	}

	l, err := virt.NewLibVirtEp("qemu:///system")
	if err != nil {
		return ""
	}
	defer l.Close()

	dom, err := l.Com.LookupDomainByName(strings.TrimSpace(*server.Metadata.InstanceName))
	if err != nil {
		return ""
	}
	defer dom.Free()

	path, err := virt.GetDomainConsolePath(dom)
	if err != nil {
		return ""
	}
	return path
}

func relayConsole(conn net.Conn, consolePath string) error {
	defer conn.Close()

	consoleDev, err := os.OpenFile(strings.TrimSpace(consolePath), os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer consoleDev.Close()

	copyErr := make(chan error, 2)
	go func() {
		_, err := io.Copy(consoleDev, conn)
		copyErr <- err
	}()
	go func() {
		_, err := io.Copy(conn, consoleDev)
		copyErr <- err
	}()

	err = <-copyErr
	if err == io.EOF || err == net.ErrClosed {
		return nil
	}
	return err
}