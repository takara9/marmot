package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentNameserverInResolvConf(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantNS  string
		wantErr bool
	}{
		{
			name:    "single nameserver",
			content: "# comment\nnameserver 127.0.0.1\noptions edns0\n",
			wantNS:  "127.0.0.1",
		},
		{
			name:    "multiple nameservers returns first",
			content: "nameserver 192.168.1.1\nnameserver 8.8.8.8\n",
			wantNS:  "192.168.1.1",
		},
		{
			name:    "marmotd generated format",
			content: "# generate by marmotd\nnameserver 192.168.122.10\noptions edns0 trust-ad\nsearch host-bridge\n",
			wantNS:  "192.168.122.10",
		},
		{
			name:    "no nameserver entry",
			content: "# only comments\n",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "resolv.conf")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(tc.content); err != nil {
				t.Fatal(err)
			}
			f.Close()

			// Temporarily override ReadFile target by writing to a known path and
			// testing the helper via a wrapper that accepts a path argument.
			data, _ := os.ReadFile(f.Name())
			got, gotErr := parseNameserverFromResolvConf(string(data))
			if tc.wantErr {
				if gotErr == nil {
					t.Fatalf("expected error but got nameserver %q", got)
				}
				return
			}
			if gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			}
			if got != tc.wantNS {
				t.Fatalf("got %q, want %q", got, tc.wantNS)
			}
		})
	}
}

func TestNameserverForDNSListenAddr(t *testing.T) {
	tests := []struct {
		name       string
		listenAddr string
		want       string
	}{
		{name: "wildcard ipv4", listenAddr: "0.0.0.0:53", want: "127.0.0.1"},
		{name: "loopback ipv4", listenAddr: "127.0.0.1:53", want: "127.0.0.1"},
		{name: "specific ipv4", listenAddr: "192.168.122.10:53", want: "192.168.122.10"},
		{name: "wildcard ipv6", listenAddr: "[::]:53", want: "::1"},
		{name: "specific ipv6", listenAddr: "[2001:db8::1]:53", want: "2001:db8::1"},
		{name: "invalid format fallback", listenAddr: "invalid", want: "127.0.0.1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nameserverForDNSListenAddr(tc.listenAddr)
			if got != tc.want {
				t.Fatalf("nameserverForDNSListenAddr(%q) = %q, want %q", tc.listenAddr, got, tc.want)
			}
		})
	}
}

func TestBackupResolvConfIfNeeded(t *testing.T) {
	t.Run("creates backup when missing", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "resolv.conf")
		bak := filepath.Join(dir, "resolv.conf.marmot.bak")
		original := "nameserver 8.8.8.8\n"

		if err := os.WriteFile(src, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}

		if err := backupResolvConfIfNeeded(src, bak); err != nil {
			t.Fatalf("backupResolvConfIfNeeded() error = %v", err)
		}

		got, err := os.ReadFile(bak)
		if err != nil {
			t.Fatalf("failed to read backup: %v", err)
		}
		if string(got) != original {
			t.Fatalf("backup content = %q, want %q", string(got), original)
		}
	})

	t.Run("does not overwrite existing backup", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "resolv.conf")
		bak := filepath.Join(dir, "resolv.conf.marmot.bak")
		existing := "nameserver 1.1.1.1\n"
		updatedSrc := "nameserver 9.9.9.9\n"

		if err := os.WriteFile(src, []byte(updatedSrc), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bak, []byte(existing), 0644); err != nil {
			t.Fatal(err)
		}

		if err := backupResolvConfIfNeeded(src, bak); err != nil {
			t.Fatalf("backupResolvConfIfNeeded() error = %v", err)
		}

		got, err := os.ReadFile(bak)
		if err != nil {
			t.Fatalf("failed to read backup: %v", err)
		}
		if string(got) != existing {
			t.Fatalf("backup was overwritten: got %q, want %q", string(got), existing)
		}
	})

	t.Run("no source file still succeeds", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "resolv.conf")
		bak := filepath.Join(dir, "resolv.conf.marmot.bak")

		if err := backupResolvConfIfNeeded(src, bak); err != nil {
			t.Fatalf("backupResolvConfIfNeeded() error = %v", err)
		}

		if _, err := os.Stat(bak); !os.IsNotExist(err) {
			t.Fatalf("backup should not be created when source is missing")
		}
	})
}
