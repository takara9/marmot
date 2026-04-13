package marmotd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

func StartMockServer(ctx context.Context, marmotPort int, etcdPort int) *Server {
	/*
		// 個別にログを確認したい場合はコメントアウトを外す
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)
	*/
	e := echo.New()
	server := NewServer("hvc", "http://127.0.0.1:"+fmt.Sprintf("%d", etcdPort))

	go func() {
		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")

		// サーバー起動
		go func() {
			if err := e.Start("0.0.0.0:" + fmt.Sprintf("%d", marmotPort)); err != nil && err != http.ErrServerClosed {
				fmt.Println("server error:", err)
			}
		}()

		// 停止シグナルを待つ
		<-ctx.Done()

		// シャットダウン
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := e.Shutdown(shutdownCtx); err != nil {
			fmt.Println("shutdown error:", err)
		}
	}()

	return server
}

func CleanupTestEnvironment() {
	cmd := exec.Command("lvremove vg1/oslv0900 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg1/oslv0901 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg1/oslv0902 -y")
	cmd.CombinedOutput()

	cmd = exec.Command("lvremove vg2/data0900 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg2/data0901 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg2/data0902 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg2/data0903 -y")
	cmd.CombinedOutput()

	cmd = exec.Command("docker kill $(docker ps |awk 'NR>1 {print $1}')")
	cmd.CombinedOutput()

	cmd = exec.Command("docker rm $(docker ps --all |awk 'NR>1 {print $1}')")
	cmd.CombinedOutput()
}

const defaultTestNetworkXML = `<network>
  <name>default</name>
  <uuid>cb75bd16-ba92-4a7f-bb3d-af50f9a1061b</uuid>
  <forward mode='nat'>
    <nat>
      <port start='1024' end='65535'/>
    </nat>
  </forward>
  <bridge name='virbr0' stp='on' delay='0'/>
  <mac address='52:54:00:62:ca:21'/>
  <ip address='192.168.122.1' netmask='255.255.255.0'>
    <dhcp>
      <range start='192.168.122.2' end='192.168.122.254'/>
    </dhcp>
  </ip>
</network>
`

func EnsureDefaultTestNetwork() error {
	cleanupLibvirtNetwork("default")

	tmpFile, err := os.CreateTemp("", "default-network-*.xml")
	if err != nil {
		return fmt.Errorf("create temp network xml: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(defaultTestNetworkXML); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp network xml: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp network xml: %w", err)
	}

	for _, args := range [][]string{{"net-define", tmpFile.Name()}, {"net-start", "default"}, {"net-autostart", "default"}} {
		cmd := exec.Command("virsh", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("virsh %s failed: %w: %s", args[0], err, string(output))
		}
	}

	return nil
}

func CleanupDefaultTestNetwork() {
	cleanupLibvirtNetwork("default")
}

func cleanupLibvirtNetwork(name string) {
	exec.Command("virsh", "net-destroy", name).CombinedOutput()
	exec.Command("virsh", "net-undefine", name).CombinedOutput()
}
