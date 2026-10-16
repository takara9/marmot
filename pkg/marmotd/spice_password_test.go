package marmotd

import (
	"strings"
	"testing"
)

// generateSpicePasswd が十分な長さ・エントロピーを持つ16進数文字列を返し、
// 連続呼び出しで異なる値を生成することを確認する。
func TestGenerateSpicePasswd(t *testing.T) {
	p1, err := generateSpicePasswd()
	if err != nil {
		t.Fatalf("generateSpicePasswd() error = %v", err)
	}
	if len(p1) != 32 {
		t.Fatalf("generateSpicePasswd() length = %d, want 32", len(p1))
	}
	for _, r := range p1 {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("generateSpicePasswd() contains non-hex character: %q", p1)
		}
	}

	p2, err := generateSpicePasswd()
	if err != nil {
		t.Fatalf("generateSpicePasswd() error = %v", err)
	}
	if p1 == p2 {
		t.Fatalf("generateSpicePasswd() returned the same value twice: %q", p1)
	}
}
