package flag

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/daffadon/waver/internal/shared"
)

var validate = validator.New()

// Handler is the interface layer for admin CRUD.
type Handler struct{ Store *Store }

type flagDTO struct {
	Key          string `json:"key" validate:"required"`
	Name         string `json:"name" validate:"required,max=120"`
	Description  string `json:"description" validate:"max=500"`
	Enabled      bool   `json:"enabled"`
	DefaultValue bool   `json:"default_value"`
	Rules        []Rule `json:"rules"`
	StartsAt     *int64 `json:"starts_at"`
	EndsAt       *int64 `json:"ends_at"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	flags, err := h.Store.List()
	if err != nil {
		slog.Error("flags list failed", "err", err)
		shared.WriteErr(w, 500, "db error")
		return
	}
	if flags == nil {
		flags = []Flag{}
	}
	shared.WriteJSON(w, 200, map[string]any{"flags": flags})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	dec := json.NewDecoder(r.Body)
	// keep raw bytes via re-marshal for presence checks
	var body json.RawMessage
	if err := dec.Decode(&body); err != nil {
		shared.WriteErr(w, 400, "bad json")
		return
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		shared.WriteErr(w, 400, "bad json")
		return
	}
	var in flagDTO
	if err := json.Unmarshal(body, &in); err != nil {
		shared.WriteErr(w, 400, "bad json")
		return
	}
	if err := validate.Struct(in); err != nil {
		shared.WriteErr(w, 400, "key and name required")
		return
	}
	if err := ValidateKey(in.Key); err != nil {
		shared.WriteErr(w, 400, err.Error())
		return
	}
	if err := ValidateRules(in.Rules); err != nil {
		shared.WriteErr(w, 400, err.Error())
		return
	}
	if err := checkSchedule(in.StartsAt, in.EndsAt); err != nil {
		shared.WriteErr(w, 400, err.Error())
		return
	}
	f := Flag{Key: in.Key, Name: in.Name, Description: in.Description, Enabled: true,
		DefaultValue: in.DefaultValue, Rules: nonNil(in.Rules), StartsAt: in.StartsAt, EndsAt: in.EndsAt}
	// Enabled defaults true when field absent.
	if _, ok := raw["enabled"]; ok {
		f.Enabled = in.Enabled
	}
	if err := h.Store.Upsert(f); err != nil {
		slog.Error("flag save failed", "key", f.Key, "err", err)
		shared.WriteErr(w, 500, "db error")
		return
	}
	saved, _ := h.Store.Get(f.Key)
	shared.WriteJSON(w, 201, saved)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	if key == "" {
		shared.WriteErr(w, 400, "key required")
		return
	}
	var in flagDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErr(w, 400, "bad json")
		return
	}
	in.Key = key
	if err := validate.Struct(in); err != nil {
		shared.WriteErr(w, 400, "name required")
		return
	}
	if err := ValidateRules(in.Rules); err != nil {
		shared.WriteErr(w, 400, err.Error())
		return
	}
	if err := checkSchedule(in.StartsAt, in.EndsAt); err != nil {
		shared.WriteErr(w, 400, err.Error())
		return
	}
	f := Flag{Key: key, Name: in.Name, Description: in.Description, Enabled: in.Enabled,
		DefaultValue: in.DefaultValue, Rules: nonNil(in.Rules), StartsAt: in.StartsAt, EndsAt: in.EndsAt}
	if err := h.Store.Upsert(f); err != nil {
		slog.Error("flag save failed", "key", key, "err", err)
		shared.WriteErr(w, 500, "db error")
		return
	}
	saved, _ := h.Store.Get(key)
	shared.WriteJSON(w, 200, saved)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	if key == "" {
		shared.WriteErr(w, 400, "key required")
		return
	}
	if err := h.Store.Delete(key); err != nil {
		slog.Error("flag delete failed", "key", key, "err", err)
		shared.WriteErr(w, 500, "db error")
		return
	}
	shared.WriteJSON(w, 200, map[string]string{"status": "deleted"})
}

func checkSchedule(starts, ends *int64) error {
	if starts != nil && ends != nil && *starts > *ends {
		return errSchedule()
	}
	return nil
}

func errSchedule() error { return &schedErr{} }

type schedErr struct{}

func (e *schedErr) Error() string { return "starts_at must be <= ends_at (UTC unix)" }

func nonNil(r []Rule) []Rule {
	if r == nil {
		return []Rule{}
	}
	return r
}
