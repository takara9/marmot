package marmotd

import (
	"runtime"
	"testing"
)

func TestNormalizeLokiPushURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "already push path", input: "http://127.0.0.1:3100/loki/api/v1/push", want: "http://127.0.0.1:3100/loki/api/v1/push"},
		{name: "base url", input: "http://127.0.0.1:3100", want: "http://127.0.0.1:3100/loki/api/v1/push"},
		{name: "base url with slash", input: "http://127.0.0.1:3100/", want: "http://127.0.0.1:3100/loki/api/v1/push"},
		{name: "custom path kept as-is", input: "http://127.0.0.1:3100/loki/api/v1/push", want: "http://127.0.0.1:3100/loki/api/v1/push"},
		{name: "arbitrary path kept as-is", input: "http://127.0.0.1:3100/custom/push", want: "http://127.0.0.1:3100/custom/push"},
		{name: "invalid", input: "127.0.0.1:3100", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeLokiPushURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected url: got=%s want=%s", got, tt.want)
			}
		})
	}
}

func TestSourceFromPC(t *testing.T) {
	pc, _, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}

	source, found := sourceFromPC(pc)
	if !found {
		t.Fatalf("source not found")
	}

	if _, ok := source["file"].(string); !ok {
		t.Fatalf("file is missing or not string: %#v", source["file"])
	}
	if _, ok := source["function"].(string); !ok {
		t.Fatalf("function is missing or not string: %#v", source["function"])
	}
	if line, ok := source["line"].(int); !ok || line <= 0 {
		t.Fatalf("line is missing or invalid: %#v", source["line"])
	}
}

func TestSourceFromPCZero(t *testing.T) {
	if _, ok := sourceFromPC(0); ok {
		t.Fatalf("expected no source for pc=0")
	}
}
