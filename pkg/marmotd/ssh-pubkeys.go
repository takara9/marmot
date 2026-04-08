package marmotd

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FetchPublicKeys は指定したURLからSSH公開鍵を取得します。
// 戻り値は公開鍵文字列のスライスです。
func FetchPublicKeys(url string) ([]string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("failed to fetch keys from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("user not found: %s", url)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d from %s", resp.StatusCode, url)
	}

	// レスポンスサイズを 64 KiB に制限（DoS対策）
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var keys []string
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			keys = append(keys, line)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no public keys found at %s", url)
	}
	return keys, nil
}
