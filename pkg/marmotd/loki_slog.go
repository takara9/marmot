package marmotd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type lokiHandler struct {
	next   slog.Handler
	attrs  []slog.Attr
	group  string
	writer *lokiWriter
}

type lokiWriter struct {
	pushURL string
	client  *http.Client
	labels  map[string]string

	mu     sync.Mutex
	closed bool
	ch     chan lokiEntry
	wg     sync.WaitGroup
}

type lokiEntry struct {
	ts   time.Time
	line string
}

type lokiPayload struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

func SetupDefaultLogger(cfg *MarmotdConfig) (func(context.Context) error, error) {
	opts := &slog.HandlerOptions{AddSource: true}
	base := slog.NewJSONHandler(os.Stderr, opts)

	if cfg == nil || strings.TrimSpace(cfg.LokiPushURL) == "" {
		slog.SetDefault(slog.New(base))
		return func(context.Context) error { return nil }, nil
	}

	writer, err := newLokiWriter(cfg)
	if err != nil {
		slog.SetDefault(slog.New(base))
		return func(context.Context) error { return nil }, err
	}

	h := &lokiHandler{next: base, writer: writer}
	slog.SetDefault(slog.New(h))

	return writer.Close, nil
}

func newLokiWriter(cfg *MarmotdConfig) (*lokiWriter, error) {
	pushURL, err := normalizeLokiPushURL(cfg.LokiPushURL)
	if err != nil {
		return nil, err
	}

	labels := map[string]string{"job": "marmotd"}
	if node := strings.TrimSpace(cfg.NodeName); node != "" {
		labels["node"] = node
	}

	w := &lokiWriter{
		pushURL: pushURL,
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
		labels: labels,
		ch:     make(chan lokiEntry, 1024),
	}

	w.wg.Add(1)
	go w.run()

	return w, nil
}

func normalizeLokiPushURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", fmt.Errorf("loki push url is empty")
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid loki push url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid loki push url: scheme and host are required")
	}

	const pushPath = "/loki/api/v1/push"
	if u.Path == "" || u.Path == "/" {
		u.Path = pushPath
	}

	return u.String(), nil
}

func (h *lokiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *lokiHandler) Handle(ctx context.Context, r slog.Record) error {
	if err := h.next.Handle(ctx, r); err != nil {
		return err
	}
	if h.writer == nil {
		return nil
	}

	body := map[string]interface{}{
		"time":  r.Time.UTC().Format(time.RFC3339Nano),
		"level": r.Level.String(),
		"msg":   r.Message,
	}

	attrs := make(map[string]interface{})
	for _, attr := range h.attrs {
		appendAttr(attrs, h.group, attr)
	}
	r.Attrs(func(attr slog.Attr) bool {
		appendAttr(attrs, h.group, attr)
		return true
	})
	if len(attrs) > 0 {
		body["attrs"] = attrs
	}

	jsonLine, err := json.Marshal(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode log for loki: %v\n", err)
		return nil
	}

	h.writer.Enqueue(r.Time, string(jsonLine))
	return nil
}

func (h *lokiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copied := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	copied = append(copied, h.attrs...)
	copied = append(copied, attrs...)
	return &lokiHandler{next: h.next.WithAttrs(attrs), writer: h.writer, attrs: copied, group: h.group}
}

func (h *lokiHandler) WithGroup(name string) slog.Handler {
	group := strings.TrimSpace(name)
	if group == "" {
		group = h.group
	} else if h.group != "" {
		group = h.group + "." + group
	}
	return &lokiHandler{next: h.next.WithGroup(name), writer: h.writer, attrs: h.attrs, group: group}
}

func appendAttr(target map[string]interface{}, group string, attr slog.Attr) {
	if attr.Equal(slog.Attr{}) {
		return
	}

	value := attr.Value.Resolve()
	key := strings.TrimSpace(attr.Key)
	if key == "" {
		return
	}
	if group != "" {
		key = group + "." + key
	}

	target[key] = attrValueToAny(value)
}

func attrValueToAny(v slog.Value) interface{} {
	switch v.Kind() {
	case slog.KindBool:
		return v.Bool()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindFloat64:
		return v.Float64()
	case slog.KindInt64:
		return v.Int64()
	case slog.KindString:
		return v.String()
	case slog.KindTime:
		return v.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindUint64:
		return v.Uint64()
	case slog.KindLogValuer:
		return attrValueToAny(v.Resolve())
	case slog.KindAny:
		return v.Any()
	default:
		return v.String()
	}
}

func (w *lokiWriter) Enqueue(ts time.Time, line string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}

	entry := lokiEntry{ts: ts, line: line}
	select {
	case w.ch <- entry:
	default:
		fmt.Fprintln(os.Stderr, "loki queue is full; dropping log entry")
	}
}

func (w *lokiWriter) run() {
	defer w.wg.Done()
	for entry := range w.ch {
		if err := w.push(entry); err != nil {
			fmt.Fprintf(os.Stderr, "failed to push log to loki: %v\n", err)
		}
	}
}

func (w *lokiWriter) push(entry lokiEntry) error {
	timestamp := entry.ts.UTC().UnixNano()
	payload := lokiPayload{
		Streams: []lokiStream{{
			Stream: w.labels,
			Values: [][2]string{{fmt.Sprintf("%d", timestamp), entry.line}},
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, w.pushURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	return nil
}

func (w *lokiWriter) Close(context.Context) error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	close(w.ch)
	w.mu.Unlock()

	w.wg.Wait()
	return nil
}
