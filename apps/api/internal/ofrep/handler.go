package ofrep

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/daffadon/waver/internal/flag"
	"github.com/daffadon/waver/internal/iam"
	"github.com/daffadon/waver/internal/shared"
)

// Handler serves OpenFeature OFREP evaluation. Auth is API key.
type Handler struct {
	Flags *flag.Store
	Keys  *iam.Store
	// Sync is optional: with peers configured it lets a caller fence its read
	// against a revision it has already seen.
	Sync Fencer
	// Wait bounds how long a fenced eval blocks before answering anyway.
	Wait time.Duration
}

// Fencer pulls replicated flags up to a revision, implemented by internal/sync.
type Fencer interface {
	CatchUp(minRev int64, wait time.Duration) bool
}

type evalReq struct {
	Context map[string]any `json:"context"`
}

// fence honours X-Waver-Min-Rev: the caller has seen at least this revision
// somewhere and wants an answer that is not older. Availability wins, so once the
// wait is up the node answers with whatever it has.
func (h *Handler) fence(r *http.Request) {
	if h.Sync == nil {
		return
	}
	raw := r.Header.Get("X-Waver-Min-Rev")
	if raw == "" {
		return
	}
	min, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || min <= 0 {
		return
	}
	wait := h.Wait
	if wait <= 0 {
		wait = time.Second
	}
	if !h.Sync.CatchUp(min, wait) {
		slog.Warn("fence not reached", "want", min, "have", h.Flags.Revision())
	}
}

func (h *Handler) Single(w http.ResponseWriter, r *http.Request) {
	if !h.authed(r) {
		slog.Warn("ofrep denied", "reason", "bad api key")
		shared.WriteErr(w, 401, "bad api key")
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/ofrep/v1/evaluate/flags/")
	if key == "" {
		shared.WriteErr(w, 400, "flag key required")
		return
	}
	h.fence(r)
	var in evalReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErr(w, 400, "bad json")
		return
	}
	if in.Context == nil || in.Context["targetingKey"] == nil {
		shared.WriteErr(w, 400, "context.targetingKey required")
		return
	}
	f, err := h.Flags.Get(key)
	if err == sql.ErrNoRows {
		shared.WriteJSON(w, 200, map[string]any{"key": key, "errorCode": "FLAG_NOT_FOUND", "errorDetails": "flag not found"})
		return
	}
	if err != nil {
		shared.WriteErr(w, 500, "db error")
		return
	}
	res := f.Eval(in.Context, time.Now().UTC())
	shared.WriteJSON(w, 200, map[string]any{"key": key, "value": res.Value, "reason": res.Reason,
		"variant": res.Variant, "metadata": map[string]any{"revision": h.Flags.Revision()}})
}

func (h *Handler) Bulk(w http.ResponseWriter, r *http.Request) {
	if !h.authed(r) {
		slog.Warn("ofrep denied", "reason", "bad api key")
		shared.WriteErr(w, 401, "bad api key")
		return
	}
	h.fence(r)
	var in evalReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErr(w, 400, "bad json")
		return
	}
	if in.Context == nil || in.Context["targetingKey"] == nil {
		shared.WriteErr(w, 400, "context.targetingKey required")
		return
	}
	flags, err := h.Flags.List()
	if err != nil {
		shared.WriteErr(w, 500, "db error")
		return
	}
	now := time.Now().UTC()
	out := make([]any, 0, len(flags))
	for _, f := range flags {
		res := f.Eval(in.Context, now)
		out = append(out, map[string]any{"key": f.Key, "value": res.Value, "reason": res.Reason, "variant": res.Variant})
	}
	shared.WriteJSON(w, 200, map[string]any{"flags": out,
		"metadata": map[string]any{"version": "v1", "revision": h.Flags.Revision()}})
}

func (h *Handler) authed(r *http.Request) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		return false
	}
	return h.Keys.ValidKey(tok)
}
