package tcp

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/daffadon/waver/internal/flag"
	"github.com/daffadon/waver/internal/iam"
)

// Server serves flag eval over raw TCP. JSON lines: one line in, one line out.
// Web stays on HTTP; this is for direct SDK clients that want no HTTP overhead.
//
//	{"type":"ping"} -> {"status":"ok"} (no auth)
//	{"api_key":"wv_...","key":"my-flag","context":{"targetingKey":"u1"}} -> single
//	{"api_key":"wv_...","context":{"targetingKey":"u1"}} -> bulk (no key)
//
// Errors are {"error":"..."} with the same strings as HTTP OFREP.
// ponytail: no TLS, plain LAN only. Add TLS or token rotation when traffic leaves VPC.
// ponytail: 1MB line cap, raise if contexts grow large.
type Server struct {
	Flags *flag.Store
	Keys  *iam.Store
}

type request struct {
	Type    string         `json:"type"`
	APIKey  string         `json:"api_key"`
	Key     string         `json:"key"`
	Context map[string]any `json:"context"`
}

// Serve listens on :port. One goroutine per conn. Blocks.
func (s *Server) Serve(port string) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	slog.Info("tcp listening", "port", port)
	for {
		c, err := ln.Accept()
		if err != nil {
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
		if len(line) > 1<<20 {
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
	if !s.Keys.ValidKey(in.APIKey) {
		return map[string]string{"error": "bad api key"}
	}
	if in.Context == nil || in.Context["targetingKey"] == nil {
		return map[string]string{"error": "context.targetingKey required"}
	}
	if in.Key != "" {
		return s.single(in.Key, in.Context)
	}
	return s.bulk(in.Context)
}

func (s *Server) single(key string, ctx map[string]any) any {
	f, err := s.Flags.Get(key)
	if err == sql.ErrNoRows {
		return map[string]any{"key": key, "errorCode": "FLAG_NOT_FOUND", "errorDetails": "flag not found"}
	}
	if err != nil {
		return map[string]string{"error": "db error"}
	}
	res := f.Eval(ctx, time.Now().UTC())
	return map[string]any{"key": key, "value": res.Value, "reason": res.Reason, "variant": res.Variant}
}

func (s *Server) bulk(ctx map[string]any) any {
	flags, err := s.Flags.List()
	if err != nil {
		return map[string]string{"error": "db error"}
	}
	now := time.Now().UTC()
	out := make([]any, 0, len(flags))
	for _, f := range flags {
		res := f.Eval(ctx, now)
		out = append(out, map[string]any{"key": f.Key, "value": res.Value, "reason": res.Reason, "variant": res.Variant})
	}
	return map[string]any{"flags": out, "metadata": map[string]string{"version": "v1"}}
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
