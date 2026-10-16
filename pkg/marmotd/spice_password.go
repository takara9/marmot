package marmotd

// Windows系VM向けグラフィカルコンソール(SPICE)のランダムパスワード生成

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// generateSpicePasswd は、SPICE 接続用のランダムパスワードを生成する。
// 固定パスワードや平文設定ファイルへの保存を避け、VM生成の都度ランダム生成する方針
// (docs/MEMO-windows-vm-support.md の決定事項)に基づく。16バイトの暗号学的乱数を
// 16進数文字列化する(32文字、XMLのpasswd属性として安全に埋め込める文字集合)。
func generateSpicePasswd() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate spice passwd: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
