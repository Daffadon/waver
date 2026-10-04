package ofrep

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
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
}

type evalReq struct {
	Context map[string]any `json:"context"`
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
	shared.WriteJSON(w, 200, map[string]any{"key": key, "value": res.Value, "reason": res.Reason, "variant": res.Variant})
}

func (h *Handler) Bulk(w http.ResponseWriter, r *http.Request) {
	if !h.authed(r) {
		slog.Warn("ofrep denied", "reason", "bad api key")
		shared.WriteErr(w, 401, "bad api key")
		return
	}
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
	shared.WriteJSON(w, 200, map[string]any{"flags": out, "metadata": map[string]string{"version": "v1"}})
}

func (h *Handler) authed(r *http.Request) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		return false
	}
	return h.Keys.ValidKey(tok)
}
