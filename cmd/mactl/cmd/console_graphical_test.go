package cmd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/takara9/marmot/pkg/client"
)

func TestPrintGraphicalConsoleInfoPrintsHostPortPasswd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/e934d/console/graphical" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"host":"192.168.1.70","port":5905,"passwd":"s3cret"}`))
	}))
	t.Cleanup(server.Close)

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	m := &client.MarmotEndpoint{
		Scheme:   parsedURL.Scheme,
		HostPort: parsedURL.Host,
		BasePath: "/api/v1",
		Client:   server.Client(),
	}

	out, _, runErr := captureStdoutAndStderr(func() error {
		return printGraphicalConsoleInfo(m, parsedURL.Host, "e934d")
	})
	if runErr != nil {
		t.Fatalf("printGraphicalConsoleInfo() returned error: %v", runErr)
	}

	if !strings.Contains(out, "host: 192.168.1.70") {
		t.Fatalf("stdout missing host line. stdout=%q", out)
	}
	if !strings.Contains(out, "port: 5905") {
		t.Fatalf("stdout missing port line. stdout=%q", out)
	}
	if !strings.Contains(out, "passwd: s3cret") {
		t.Fatalf("stdout missing passwd line. stdout=%q", out)
	}
	if !strings.Contains(out, "remote-viewer spice://192.168.1.70:5905") {
		t.Fatalf("stdout missing remote-viewer hint. stdout=%q", out)
	}
}

// passwd が未設定(省略)の場合、passwd 行自体を出力しないことを確認する。
func TestPrintGraphicalConsoleInfoOmitsPasswdLineWhenAbsent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"host":"127.0.0.1","port":5900}`))
	}))
	t.Cleanup(server.Close)

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	m := &client.MarmotEndpoint{
		Scheme:   parsedURL.Scheme,
		HostPort: parsedURL.Host,
		BasePath: "/api/v1",
		Client:   server.Client(),
	}

	out, _, runErr := captureStdoutAndStderr(func() error {
		return printGraphicalConsoleInfo(m, parsedURL.Host, "abc12")
	})
	if runErr != nil {
		t.Fatalf("printGraphicalConsoleInfo() returned error: %v", runErr)
	}
	if strings.Contains(out, "passwd:") {
		t.Fatalf("stdout should not contain passwd line when absent. stdout=%q", out)
	}
}
