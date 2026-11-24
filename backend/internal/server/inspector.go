package server

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"mockdigilocker/internal/model"
	"mockdigilocker/internal/security"
)

// ReqInfo is per-request metadata that handlers enrich for the inspector.
type ReqInfo struct {
	ID            string
	TraceID       string
	SpanID        string
	ParentSpanID  string
	CorrelationID string
	Category      string
	Endpoint      string
	ClientID      string
	UserID        string
	Scenario      string
	Error         string
	Rewritten     string
	Notes         []string
}

func (ri *ReqInfo) Note(s string) { ri.Notes = append(ri.Notes, s) }

// RequestLog is a captured request/response pair shown in the API inspector.
type RequestLog struct {
	ID              string              `json:"id"`
	Timestamp       time.Time           `json:"timestamp"`
	Category        string              `json:"category"`
	Endpoint        string              `json:"endpoint,omitempty"`
	Method          string              `json:"method"`
	Path            string              `json:"path"`
	Query           string              `json:"query,omitempty"`
	Rewritten       string              `json:"rewritten,omitempty"`
	RemoteIP        string              `json:"remote_ip"`
	RequestHeaders  map[string][]string `json:"request_headers"`
	RequestBody     string              `json:"request_body,omitempty"`
	Status          int                 `json:"status"`
	ResponseHeaders map[string][]string `json:"response_headers"`
	ResponseBody    string              `json:"response_body,omitempty"`
	LatencyMs       float64             `json:"latency_ms"`
	TraceID         string              `json:"trace_id"`
	SpanID          string              `json:"span_id"`
	CorrelationID   string              `json:"correlation_id"`
	ClientID        string              `json:"client_id,omitempty"`
	UserID          string              `json:"user_id,omitempty"`
	Scenario        string              `json:"scenario,omitempty"`
	Error           string              `json:"error,omitempty"`
	Notes           []string            `json:"notes,omitempty"`
}

type captureWriter struct {
	http.ResponseWriter
	status   int
	buf      bytes.Buffer
	max      int
	size     int
	hijacked bool
}

func (c *captureWriter) WriteHeader(s int) {
	if c.status == 0 {
		c.status = s
	}
	c.ResponseWriter.WriteHeader(s)
}

func (c *captureWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = 200
	}
	if c.buf.Len() < c.max {
		n := min(len(b), c.max-c.buf.Len())
		c.buf.Write(b[:n])
	}
	c.size += len(b)
	return c.ResponseWriter.Write(b)
}

func (c *captureWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *captureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := c.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	c.hijacked = true
	return h.Hijack()
}

func (c *captureWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

func categorize(path string) string {
	switch {
	case strings.HasPrefix(path, "/public/"), strings.HasPrefix(path, "/.well-known/"):
		return "api"
	case strings.HasPrefix(path, "/oauth/"):
		return "consent-ui"
	case strings.HasPrefix(path, "/portal-api/"):
		return "portal"
	case strings.HasPrefix(path, "/admin-api/"):
		return "admin"
	case strings.HasPrefix(path, "/demo-partner"):
		return "demo-partner"
	}
	return ""
}

// inspect assigns trace/correlation ids, captures traffic for the inspector
// and emits an OpenTelemetry span per request.
func (a *App) inspect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ri := &ReqInfo{ID: security.RandomHex(8), Category: categorize(r.URL.Path)}
		ri.TraceID, ri.ParentSpanID = parseTraceparent(r.Header.Get("traceparent"))
		if ri.TraceID == "" {
			ri.TraceID = security.RandomHex(16)
		}
		ri.SpanID = security.RandomHex(8)
		ri.CorrelationID = firstHeader(r, "X-Correlation-ID", "X-Request-ID")
		if ri.CorrelationID == "" {
			ri.CorrelationID = ri.TraceID
		}
		w.Header().Set("traceparent", "00-"+ri.TraceID+"-"+ri.SpanID+"-01")
		w.Header().Set("X-Trace-Id", ri.TraceID)
		w.Header().Set("X-Correlation-ID", ri.CorrelationID)
		if ri.Category == "api" || ri.Category == "consent-ui" {
			w.Header().Set("X-Sandbox", "mock-digilocker; synthetic-data-only")
		}
		r = r.WithContext(withReqInfo(r.Context(), ri))

		logIt := ri.Category != "" && !(ri.Category == "admin" && r.Method == http.MethodGet)
		var reqBody []byte
		if logIt && r.Body != nil && r.ContentLength != 0 {
			reqBody, _ = io.ReadAll(io.LimitReader(r.Body, 11<<20))
			r.Body = io.NopCloser(bytes.NewReader(reqBody))
		}
		cw := &captureWriter{ResponseWriter: w, max: a.cfg.LogBodyMax}
		next.ServeHTTP(cw, r)
		if cw.status == 0 && !cw.hijacked {
			cw.status = 200
		}
		lat := time.Since(start)
		a.tracer.Record(Span{TraceID: ri.TraceID, SpanID: ri.SpanID, Parent: ri.ParentSpanID,
			Name: r.Method + " " + spanName(r.URL.Path), Start: start, End: time.Now(), Status: cw.status,
			Attrs: map[string]string{"http.method": r.Method, "http.target": r.URL.Path, "http.status_code": itoa(cw.status),
				"mockdl.client_id": ri.ClientID, "mockdl.user_id": ri.UserID, "mockdl.scenario": ri.Scenario, "mockdl.correlation_id": ri.CorrelationID}})
		if !logIt {
			return
		}
		status := cw.status
		if cw.hijacked {
			status = 0
			ri.Note("connection closed without response (hijacked)")
		}
		entry := RequestLog{
			ID: ri.ID, Timestamp: start.UTC(), Category: ri.Category, Endpoint: ri.Endpoint, Method: r.Method, Path: r.URL.Path,
			Query: r.URL.RawQuery, Rewritten: ri.Rewritten, RemoteIP: clientIP(r), RequestHeaders: r.Header.Clone(),
			RequestBody: a.bodyForLog(reqBody, r.Header.Get("Content-Type")), Status: status,
			ResponseHeaders: cw.Header().Clone(), ResponseBody: a.bodyForLogSized(cw.buf.Bytes(), cw.size, cw.Header().Get("Content-Type")),
			LatencyMs: float64(lat.Microseconds()) / 1000, TraceID: ri.TraceID, SpanID: ri.SpanID, CorrelationID: ri.CorrelationID,
			ClientID: ri.ClientID, UserID: ri.UserID, Scenario: ri.Scenario, Error: ri.Error, Notes: ri.Notes,
		}
		go func() {
			_ = a.st.Put(model.CollRequestLog, entry.ID, entry)
			_ = a.st.Trim(model.CollRequestLog, a.cfg.RequestLogMax)
		}()
	})
}

func spanName(p string) string {
	// collapse document URIs to keep span names low-cardinality
	for _, pre := range []string{"/file/", "/xml/", "/files/"} {
		if i := strings.Index(p, pre); i >= 0 && !strings.HasSuffix(p, "/issued") && !strings.HasSuffix(p, "/eaadhaar") {
			return p[:i+len(pre)] + "{uri}"
		}
	}
	return p
}

func firstHeader(r *http.Request, names ...string) string {
	for _, n := range names {
		if v := r.Header.Get(n); v != "" {
			if len(v) > 128 {
				v = v[:128]
			}
			return v
		}
	}
	return ""
}

func (a *App) bodyForLog(b []byte, ct string) string { return a.bodyForLogSized(b, len(b), ct) }

func (a *App) bodyForLogSized(b []byte, total int, ct string) string {
	if total == 0 {
		return ""
	}
	ct = strings.ToLower(ct)
	textual := strings.Contains(ct, "json") || strings.Contains(ct, "xml") || strings.Contains(ct, "text") ||
		strings.Contains(ct, "x-www-form-urlencoded") || strings.Contains(ct, "javascript") || ct == ""
	if !textual || !utf8.Valid(b) {
		return "<binary " + itoa(total) + " bytes, " + ct + ">"
	}
	if len(b) > a.cfg.LogBodyMax {
		b = b[:a.cfg.LogBodyMax]
	}
	s := string(b)
	if total > len(b) {
		s += "\n…<truncated, " + itoa(total) + " bytes total>"
	}
	return s
}

// parseTraceparent parses a W3C traceparent header.
func parseTraceparent(h string) (traceID, parentID string) {
	parts := strings.Split(strings.TrimSpace(h), "-")
	if len(parts) != 4 || len(parts[1]) != 32 || len(parts[2]) != 16 {
		return "", ""
	}
	if strings.Trim(parts[1], "0") == "" || !isHex(parts[1]) || !isHex(parts[2]) {
		return "", ""
	}
	return strings.ToLower(parts[1]), strings.ToLower(parts[2])
}

func isHex(s string) bool {
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
