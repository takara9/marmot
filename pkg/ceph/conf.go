package ceph

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ParseConnectionFromConf(confPath, keyringPath string) ([]string, string, error) {
	monitors, confUser, confKeyring, err := parseCephConf(confPath)
	if err != nil {
		return nil, "", err
	}
	if len(monitors) == 0 {
		return nil, "", fmt.Errorf("ceph monitors are required")
	}

	effectiveKeyring := strings.TrimSpace(keyringPath)
	if effectiveKeyring == "" {
		effectiveKeyring = strings.TrimSpace(confKeyring)
	}
	if effectiveKeyring == "" {
		return nil, "", fmt.Errorf("ceph keyring file is required")
	}

	user := normalizeCephUser(confUser)
	if user == "" {
		user = parseUserFromKeyringFile(effectiveKeyring)
	}
	if user == "" {
		user = parseUserFromKeyringPath(effectiveKeyring)
	}
	if user == "" {
		user = "admin"
	}

	return monitors, user, nil
}

func parseCephConf(confPath string) ([]string, string, string, error) {
	f, err := os.Open(strings.TrimSpace(confPath))
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to open ceph conf: %w", err)
	}
	defer func() { _ = f.Close() }()

	var (
		monitors []string
		user     string
		keyring  string
	)
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := sanitizeConfLine(s.Text())
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		k, v, ok := splitKV(line)
		if !ok {
			continue
		}
		switch normalizeKey(k) {
		case "mon_host":
			monitors = append(monitors, parseMonHosts(v)...)
		case "name", "user", "client":
			if user == "" {
				user = normalizeCephUser(v)
			}
		case "keyring":
			if keyring == "" {
				keyring = strings.TrimSpace(trimQuote(v))
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, "", "", fmt.Errorf("failed to read ceph conf: %w", err)
	}
	return uniqueNonEmpty(monitors), user, keyring, nil
}

func sanitizeConfLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return ""
	}
	if idx := strings.IndexAny(line, "#;"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	return line
}

func splitKV(line string) (string, string, bool) {
	idx := strings.Index(line, "=")
	if idx < 0 {
		return "", "", false
	}
	k := strings.TrimSpace(line[:idx])
	v := strings.TrimSpace(line[idx+1:])
	if k == "" || v == "" {
		return "", "", false
	}
	return k, v, true
}

func normalizeKey(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	k = strings.ReplaceAll(k, "-", "_")
	k = strings.ReplaceAll(k, " ", "_")
	return k
}

func parseMonHosts(value string) []string {
	value = strings.TrimSpace(trimQuote(value))
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "v1:") || strings.HasPrefix(p, "v2:") {
			p = p[3:]
		}
		if idx := strings.Index(p, "/"); idx >= 0 {
			p = p[:idx]
		}
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizeCephUser(value string) string {
	v := strings.TrimSpace(trimQuote(value))
	v = strings.TrimPrefix(v, "client.")
	return v
}

func parseUserFromKeyringFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
			continue
		}
		section := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
		section = strings.TrimSpace(section)
		if strings.HasPrefix(section, "client.") {
			return strings.TrimPrefix(section, "client.")
		}
	}
	return ""
}

func parseUserFromKeyringPath(path string) string {
	base := filepath.Base(strings.TrimSpace(path))
	base = strings.TrimSuffix(base, ".keyring")
	if strings.HasPrefix(base, "ceph.client.") {
		return strings.TrimPrefix(base, "ceph.client.")
	}
	return ""
}

func trimQuote(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "\"")
	value = strings.TrimSuffix(value, "\"")
	return value
}

func uniqueNonEmpty(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
