package iam

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/daffadon/waver/internal/shared"
)

var validate = validator.New()

// Handler is the interface layer. Service logic stays here (thin app layer).
type Handler struct {
	Store *Store
	Cfg   shared.Config
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password" validate:"required,min=1"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErr(w, 400, "bad json")
		return
	}
	if err := validate.Struct(in); err != nil {
		shared.WriteErr(w, 400, "password required")
		return
	}
	admin, found, err := h.Store.GetAdmin()
	if err != nil || !found {
		slog.Error("login failed", "reason", "admin not seeded", "err", err)
		shared.WriteErr(w, 500, "admin not seeded")
		return
	}
	if !CheckPassword(admin.PasswordHash, in.Password) {
		slog.Warn("login failed", "reason", "bad credentials")
		shared.WriteErr(w, 401, "bad credentials")
		return
	}
	if admin.MustChange {
		slog.Info("login blocked", "reason", "password change required")
		shared.WriteErr(w, 403, "MUST_CHANGE")
		return
	}
	tok, err := MintJWT(h.Cfg.JWTSecret, "admin")
	if err != nil {
		slog.Error("login failed", "reason", "cannot mint token", "err", err)
		shared.WriteErr(w, 500, "cannot mint token")
		return
	}
	slog.Info("login ok")
	shared.WriteJSON(w, 200, map[string]string{"token": tok})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current" validate:"required"`
		Next    string `json:"next" validate:"required,min=12,max=128"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErr(w, 400, "bad json")
		return
	}
	if err := validate.Struct(in); err != nil {
		shared.WriteErr(w, 400, "next must be 12-128 chars")
		return
	}
	admin, _, err := h.Store.GetAdmin()
	if err != nil {
		shared.WriteErr(w, 500, "db error")
		return
	}
	// First-login flow has no JWT yet: allow when must_change and current matches.
	// A present-but-invalid JWT (expired, rotated secret) is told to sign in again.
	raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	authed := IsAdmin(r, h.Cfg.JWTSecret)
	switch {
	case !authed && raw != "":
		slog.Warn("change-password denied", "reason", "bad jwt")
		shared.WriteErr(w, 401, "session expired, sign in again")
		return
	case !authed && !(admin.MustChange && CheckPassword(admin.PasswordHash, in.Current)):
		slog.Warn("change-password denied", "reason", "wrong current or already changed")
		shared.WriteErr(w, 401, "wrong current password")
		return
	case authed && !CheckPassword(admin.PasswordHash, in.Current):
		slog.Warn("change-password denied", "reason", "wrong current password")
		shared.WriteErr(w, 401, "wrong current password")
		return
	}
	hash, err := HashPassword(in.Next)
	if err != nil {
		slog.Error("change-password failed", "reason", "hash failed", "err", err)
		shared.WriteErr(w, 500, "hash failed")
		return
	}
	if err := h.Store.PutAdmin(hash, false); err != nil {
		slog.Error("change-password failed", "reason", "db error", "err", err)
		shared.WriteErr(w, 500, "db error")
		return
	}
	slog.Info("password changed")
	shared.WriteJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) CreateKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name" validate:"required,min=1,max=64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErr(w, 400, "bad json")
		return
	}
	if err := validate.Struct(in); err != nil {
		shared.WriteErr(w, 400, "name 1-64 chars")
		return
	}
	id, prefix, plain, err := RandomKey()
	if err != nil {
		shared.WriteErr(w, 500, "rand failed")
		return
	}
	k := APIKey{ID: id, Prefix: prefix, Name: in.Name, CreatedAt: time.Now().UTC().Unix()}
	if err := h.Store.InsertKey(k, KeyHash(plain)); err != nil {
		slog.Error("apikey create failed", "reason", "db error", "err", err)
		shared.WriteErr(w, 500, "db error")
		return
	}
	slog.Info("apikey created", "prefix", prefix)
	shared.WriteJSON(w, 201, map[string]any{"id": id, "prefix": prefix, "name": in.Name, "key": plain})
}

func (h *Handler) ListKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.Store.ListKeys()
	if err != nil {
		shared.WriteErr(w, 500, "db error")
		return
	}
	if keys == nil {
		keys = []APIKey{}
	}
	shared.WriteJSON(w, 200, map[string]any{"keys": keys})
}

func (h *Handler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/apikeys/")
	if id == "" {
		shared.WriteErr(w, 400, "id required")
		return
	}
	if err := h.Store.RevokeKey(id); err != nil {
		slog.Error("apikey revoke failed", "reason", "db error", "err", err)
		shared.WriteErr(w, 500, "db error")
		return
	}
	slog.Info("apikey revoked", "id", id)
	shared.WriteJSON(w, 200, map[string]string{"status": "revoked"})
}

func IsAdmin(r *http.Request, secret string) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" || secret == "" {
		return false
	}
	return ParseJWT(secret, tok)
}
