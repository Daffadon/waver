package tcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/daffadon/waver/internal/sync"
)

// Server serves the whole HTTP API over raw TCP. JSON lines: one line in, one line out.
// Every line becomes an http.Request run through the same router as HTTP, so auth,
// validation and error strings never drift.
//
//	{"type":"ping"} -> {"status":"ok"} (no auth)
//
// Eval is sugar for OFREP, auth by API key:
//
//	{"api_key":"wv_...","key":"my-flag","context":{"targetingKey":"u1"}} -> single
//	{"api_key":"wv_...","context":{"targetingKey":"u1"}} -> bulk (no key)
//
// Any other route passes method/path/body straight through:
//
//	{"method":"GET","path":"/api/v1/flags","token":"<jwt>"}
//	{"method":"POST","path":"/api/v1/flags","token":"<jwt>","body":{...}}
//
// token is the admin JWT for /api/v1/*; api_key is the key for /ofrep/*.
//
// Replication frames share this port and are routed to Sync, authenticated by
// their own MAC instead of a JWT:
//
//	{"type":"sync_digest",...} -> {"type":"sync_digest_reply",...}
//
// min_revision on an eval line becomes the X-Waver-Min-Rev fence:
//
//	{"api_key":"wv_...","key":"my-flag","context":{...},"min_revision":42}
//
// ponytail: admin JWT and login passwords now cross plaintext TCP. Keep TCP on a
// trusted LAN; add TLS or disable admin routes on TCP before this leaves the VPC.
type Server struct {
	Handler http.Handler
	// Sync handles replication frames. Nil means this node is single-node and
	// replication lines are refused.
	Sync Sync
}

// Sync is the replication half of the TCP port, implemented by internal/sync.
type Sync interface {
	// Frame handles one inbound replication line, reporting whether the line was
	// a replication frame at all.
	Frame(raw []byte) (any, bool)
}

// maxLine caps one protocol line. A state push carries every flag, so this is
// the ceiling on a full replication frame.
const maxLine = 8 << 20

type request struct {
	Type    string          `json:"type"`
	Method  string          `json:"method"`
	Path    string          `json:"path"`
	Token   string          `json:"token"`
	APIKey  string          `json:"api_key"`
	Key     string          `json:"key"`
	Context map[string]any  `json:"context"`
	Body    json.RawMessage `json:"body"`
	MinRev  int64           `json:"min_revision"`
}

// Serve listens on :port. One goroutine per conn. Blocks.
func (s *Server) Serve(port string) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	slog.Info("tcp listening", "port", port)
	return s.ServeOn(ln)
}

// ServeOn serves an already open listener, which tests use to pick a port.
func (s *Server) ServeOn(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			slog.Warn("tcp accept failed", "err", err)
			continue
		}
		go s.handle(c)
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	defer w.Flush()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if len(line) > maxLine {
			_ = writeJSON(w, map[string]string{"error": "line too large"})
			_ = w.Flush()
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		_ = writeJSON(w, s.handleLine([]byte(line)))
		_ = w.Flush()
	}
}

func (s *Server) handleLine(line []byte) any {
	var in request
	if err := json.Unmarshal(line, &in); err != nil {
		return map[string]string{"error": "bad json"}
	}
	if in.Type == "ping" {
		return map[string]string{"status": "ok"}
	}
	if sync.IsFrame(in.Type) {
		if s.Sync == nil {
			return map[string]string{"error": "replication disabled"}
		}
		resp, _ := s.Sync.Frame(line)
		return resp
	}
	method, path, body, bearer := route(in)
	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return map[string]string{"error": "bad request"}
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if in.MinRev > 0 {
		req.Header.Set("X-Waver-Min-Rev", strconv.FormatInt(in.MinRev, 10))
	}
	rec := &recorder{header: http.Header{}, code: http.StatusOK}
	s.Handler.ServeHTTP(rec, req)
	return rec.result()
}

// route maps a line to an HTTP request path. The old eval shapes are sugar
// for the OFREP endpoints.
func route(in request) (method, path string, body io.Reader, bearer string) {
	if in.Path == "" {
		b, _ := json.Marshal(map[string]any{"context": in.Context})
		if in.Key != "" {
			return http.MethodPost, "/ofrep/v1/evaluate/flags/" + in.Key, bytes.NewReader(b), in.APIKey
		}
		return http.MethodPost, "/ofrep/v1/evaluate/flags", bytes.NewReader(b), in.APIKey
	}
	method = in.Method
	if method == "" {
		method = http.MethodGet
	}
	if len(in.Body) > 0 && string(in.Body) != "null" {
		body = bytes.NewReader(in.Body)
	}
	return method, in.Path, body, firstNonEmpty(in.Token, in.APIKey)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// recorder captures a handler response so the body can go back down the line.
type recorder struct {
	header http.Header
	code   int
	buf    bytes.Buffer
}

func (r *recorder) Header() http.Header         { return r.header }
func (r *recorder) WriteHeader(code int)        { r.code = code }
func (r *recorder) Write(b []byte) (int, error) { return r.buf.Write(b) }

// result returns the handler body verbatim when it is JSON, so eval and error
// payloads match HTTP exactly. Non-JSON (static files, docs) is wrapped.
func (r *recorder) result() any {
	b := bytes.TrimSpace(r.buf.Bytes())
	if len(b) > 0 && json.Valid(b) {
		return json.RawMessage(b)
	}
	return map[string]any{"status": r.code, "body": r.buf.String()}
}

func writeJSON(w *bufio.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		b = []byte(`{"error":"encode error"}`)
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}
