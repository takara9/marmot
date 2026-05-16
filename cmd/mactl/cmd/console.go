package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"golang.org/x/term"
)

var errConsoleDetach = errors.New("console detached")

var consoleCmd = &cobra.Command{
	Use:   "console SERVER-NAME",
	Short: "Connect to a server console",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		m, err := getClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get API client config: %w", err)
		}

		serverName := strings.TrimSpace(args[0])
		if serverName == "" {
			return fmt.Errorf("server name is required")
		}

		list, _, err := m.GetServers()
		if err != nil {
			return fmt.Errorf("failed to list servers: %w", err)
		}

		var servers []api.Server
		if err := json.Unmarshal(list, &servers); err != nil {
			return fmt.Errorf("failed to parse servers: %w", err)
		}

		var server *api.Server
		for i := range servers {
			if strings.TrimSpace(servers[i].Metadata.Name) == serverName {
				server = &servers[i]
				break
			}
		}
		if server == nil {
			return fmt.Errorf("server %q not found", serverName)
		}

		hostPort := strings.TrimSpace(m.HostPort)
		if hostPort == "" {
			return fmt.Errorf("API host is required")
		}
		consolePath := strings.TrimSpace(m.BasePath)
		if consolePath == "" {
			consolePath = "/api/v1"
		}
		serverID := api.ServerID(*server)
		requestPath := strings.TrimRight(consolePath, "/") + "/server/" + serverID + "/console"

		timeout := 0 * time.Second
		if m.Client != nil && m.Client.Timeout > 0 {
			timeout = m.Client.Timeout
		}
		conn, err := net.DialTimeout("tcp", hostPort, timeout)
		if err != nil {
			return fmt.Errorf("failed to connect to marmotd: %w", err)
		}
		defer conn.Close()

		req, err := http.NewRequest(http.MethodGet, requestPath, nil)
		if err != nil {
			return fmt.Errorf("failed to build console request: %w", err)
		}
		req.Host = hostPort
		req.Header.Set("Connection", "close")
		if err := req.Write(conn); err != nil {
			return fmt.Errorf("failed to send console request: %w", err)
		}

		reader := bufio.NewReader(conn)
		resp, err := http.ReadResponse(reader, req)
		if err != nil {
			return fmt.Errorf("failed to read console response: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			message := strings.TrimSpace(string(body))
			if message == "" {
				message = resp.Status
			}
			return fmt.Errorf("console request failed: %s", message)
		}

		if term.IsTerminal(int(os.Stdin.Fd())) {
			oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
			if err != nil {
				return fmt.Errorf("failed to enable raw terminal: %w", err)
			}
			defer func() {
				_ = term.Restore(int(os.Stdin.Fd()), oldState)
				fmt.Fprint(os.Stdout, "\r\n")
			}()
		}

		errorCh := make(chan error, 2)
		go func() {
			errorCh <- copyConsoleInput(conn, os.Stdin)
		}()
		go func() {
			_, err := io.Copy(os.Stdout, reader)
			errorCh <- err
		}()

		err = <-errorCh
		if errors.Is(err, errConsoleDetach) {
			return nil
		}
		if err != nil && err != io.EOF && err != net.ErrClosed {
			return err
		}
		return nil
	},
}

func copyConsoleInput(dst io.Writer, src io.Reader) error {
	buf := make([]byte, 1)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if buf[0] == 0x1d {
				return errConsoleDetach
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				if werr == net.ErrClosed {
					return nil
				}
				return werr
			}
		}
		if err != nil {
			if err == io.EOF || err == net.ErrClosed {
				return nil
			}
			return err
		}
	}
}

func init() {
	rootCmd.AddCommand(consoleCmd)
}