package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Span is a finished server span.
type Span struct {
	TraceID, SpanID, Parent, Name string
	Start, End                    time.Time
	Status                        int
	Attrs                         map[string]string
}

// Tracer exports spans with OTLP/HTTP (JSON encoding) to any OpenTelemetry
// collector, Jaeger or Tempo. Disabled when no endpoint is configured; trace
// and correlation ids are still generated, propagated (W3C traceparent) and
// shown in the request inspector.
type Tracer struct {
	endpoint string
	service  string
	ch       chan Span
	done     chan struct{}
	log      *slog.Logger
	client   *http.Client
}

func NewTracer(endpoint, service string, log *slog.Logger) *Tracer {
	t := &Tracer{endpoint: strings.TrimRight(endpoint, "/"), service: service, log: log, done: make(chan struct{}),
		client: &http.Client{Timeout: 5 * time.Second}}
	if t.endpoint != "" {
		t.ch = make(chan Span, 4096)
		go t.loop()
	}
	return t
}

func (t *Tracer) Record(s Span) {
	if t.ch == nil {
		return
	}
	select {
	case t.ch <- s:
	default: // drop when the exporter falls behind
	}
}

func (t *Tracer) loop() {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	var batch []Span
	flush := func() {
		if len(batch) > 0 {
			t.export(batch)
			batch = nil
		}
	}
	for {
		select {
		case s := <-t.ch:
			batch = append(batch, s)
			if len(batch) >= 256 {
				flush()
			}
		case <-tick.C:
			flush()
		case <-t.done:
			flush()
			return
		}
	}
}

func (t *Tracer) export(spans []Span) {
	type kv struct {
		Key   string         `json:"key"`
		Value map[string]any `json:"value"`
	}
	out := make([]map[string]any, 0, len(spans))
	for _, s := range spans {
		var attrs []kv
		for k, v := range s.Attrs {
			if v != "" {
				attrs = append(attrs, kv{k, map[string]any{"stringValue": v}})
			}
		}
		code := 1
		if s.Status >= 500 || s.Status == 0 {
			code = 2
		}
		sp := map[string]any{
			"traceId": s.TraceID, "spanId": s.SpanID, "name": s.Name, "kind": 2,
			"startTimeUnixNano": strconv.FormatInt(s.Start.UnixNano(), 10), "endTimeUnixNano": strconv.FormatInt(s.End.UnixNano(), 10),
			"attributes": attrs, "status": map[string]any{"code": code},
		}
		if s.Parent != "" {
			sp["parentSpanId"] = s.Parent
		}
		out = append(out, sp)
	}
	body, _ := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{
		"resource":   map[string]any{"attributes": []kv{{"service.name", map[string]any{"stringValue": t.service}}}},
		"scopeSpans": []any{map[string]any{"scope": map[string]any{"name": "mock-digilocker"}, "spans": out}},
	}}})
	resp, err := t.client.Post(t.endpoint+"/v1/traces", "application/json", bytes.NewReader(body))
	if err != nil {
		t.log.Debug("otlp export failed", "err", err)
		return
	}
	resp.Body.Close()
}

func (t *Tracer) Close() {
	if t.ch != nil {
		close(t.done)
	}
}
