package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/controller"
	internaldns "github.com/takara9/marmot/pkg/internal-dns"
	"github.com/takara9/marmot/pkg/marmotd"
)

type mockServerHandle struct {
	server *marmotd.Server
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func startMockServer() (*mockServerHandle, error) {
	// 個別にログを確認したい場合はコメントアウトを外す
	//opts := &slog.HandlerOptions{
	//	AddSource: true,
	//	Level:     slog.LevelDebug,
	//}
	//logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	//slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	h := &mockServerHandle{
		cancel: cancel,
		done:   make(chan struct{}),
	}

	nodeName := "hvc"
	etcdEp := "http://127.0.0.1:3379"

	e := echo.New()
	server := marmotd.NewServer(nodeName, etcdEp)
	h.server = server

	readyCh := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		defer close(h.done)
		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")

		type stopper interface{ Stop() }
		stoppers := make([]stopper, 0, 6)
		stopControllers := func() {
			for i := len(stoppers) - 1; i >= 0; i-- {
				stoppers[i].Stop()
			}
		}

		// コントローラーの開始
		vmController, err := controller.StartVmController(nodeName, etcdEp, 0)
		if err != nil {
			slog.Error("Failed to start VM controller", "err", err)
			errCh <- err
			return
		}
		stoppers = append(stoppers, vmController)

		volController, err := controller.StartVolController(nodeName, etcdEp, 0)
		if err != nil {
			slog.Error("Failed to start volume controller", "err", err)
			stopControllers()
			errCh <- err
			return
		}
		stoppers = append(stoppers, volController)

		netController, err := controller.StartNetController(nodeName, etcdEp, 0)
		if err != nil {
			slog.Error("Failed to start network controller", "err", err)
			stopControllers()
			errCh <- err
			return
		}
		stoppers = append(stoppers, netController)

		_, err = internaldns.StartInternalDNSServer(ctx, nodeName, etcdEp, nil)
		if err != nil {
			slog.Error("Failed to start DNS server", "err", err)
			stopControllers()
			errCh <- err
			return
		}

		imageController, err := controller.StartImageController(nodeName, etcdEp, 0)
		if err != nil {
			slog.Error("Failed to start image controller", "err", err)
			stopControllers()
			errCh <- err
			return
		}
		stoppers = append(stoppers, imageController)

		hostController, err := controller.StartHostController(nodeName, etcdEp)
		if err != nil {
			slog.Error("Failed to start host controller", "err", err)
			stopControllers()
			errCh <- err
			return
		}
		stoppers = append(stoppers, hostController)

		schedulerController, err := controller.StartSchedulerController(nodeName, etcdEp)
		if err != nil {
			slog.Error("Failed to start scheduler controller", "err", err)
			stopControllers()
			errCh <- err
			return
		}
		stoppers = append(stoppers, schedulerController)

		serverErrCh := make(chan error, 1)
		go func() {
			serverErrCh <- e.Start("0.0.0.0:8080")
		}()

		close(readyCh)

		<-ctx.Done()
		stopControllers()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.Shutdown(shutdownCtx); err != nil {
			fmt.Println("shutdown error:", err)
		}

		select {
		case err := <-serverErrCh:
			if err != nil && err != http.ErrServerClosed {
				fmt.Println("server error:", err)
			}
		default:
		}
	}()

	select {
	case <-readyCh:
	case err := <-errCh:
		cancel()
		<-h.done
		return nil, fmt.Errorf("mock server startup failed: %w", err)
	case <-time.After(10 * time.Second):
		cancel()
		<-h.done
		return nil, errors.New("mock server startup timeout")
	}

	if err := waitMockServerReady(30 * time.Second); err != nil {
		h.Stop()
		return nil, err
	}

	return h, nil
}

func (h *mockServerHandle) Stop() {
	if h == nil {
		return
	}
	h.once.Do(func() {
		if h.cancel != nil {
			h.cancel()
		}
	})
	if h.done != nil {
		<-h.done
	}
}

func waitMockServerReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}
	readinessURL := "http://127.0.0.1:8080/api/v1/ping"

	for time.Now().Before(deadline) {
		resp, err := client.Get(readinessURL)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return errors.New("mock server readiness timeout")
}

func cleanupTestEnvironment() {
	// データのクリア
}
