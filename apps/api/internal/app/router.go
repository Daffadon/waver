package app

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/daffadon/waver/internal/flag"
	"github.com/daffadon/waver/internal/iam"
	"github.com/daffadon/waver/internal/ofrep"
	"github.com/daffadon/waver/internal/shared"
	"github.com/swaggest/swgui/v5emb"
)

// Router wires admin API, OFREP, and static web dist from disk.
func Router(cfg shared.Config, iamH *iam.Handler, flagH *flag.Handler, ofrepH *ofrep.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/auth/login", iamH.Login)
	mux.HandleFunc("/api/v1/auth/change-password", iamH.ChangePassword)
	mux.HandleFunc("/api/v1/apikeys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			iamH.ListKeys(w, r)
		case "POST":
			iamH.CreateKey(w, r)
		default:
			shared.WriteErr(w, 405, "method not allowed")
		}
	})
	mux.HandleFunc("/api/v1/apikeys/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			iamH.RevokeKey(w, r)
			return
		}
		shared.WriteErr(w, 405, "method not allowed")
	})

	mux.HandleFunc("/api/v1/flags", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			flagH.List(w, r)
		case "POST":
			flagH.Create(w, r)
		default:
			shared.WriteErr(w, 405, "method not allowed")
		}
	})
	mux.HandleFunc("/api/v1/flags/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PATCH", "PUT":
			flagH.Update(w, r)
		case "DELETE":
			flagH.Delete(w, r)
		default:
			shared.WriteErr(w, 405, "method not allowed")
		}
	})

	mux.HandleFunc("/ofrep/v1/evaluate/flags", ofrepH.Bulk)
	mux.HandleFunc("/ofrep/v1/evaluate/flags/", ofrepH.Single)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		shared.WriteJSON(w, 200, map[string]string{"status": "ok"})
	})

	// Public client reference: raw contract plus Swagger UI.
	mux.HandleFunc("/docs/openapi.yaml", serveSpec)
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusFound)
	})
	mux.Handle("/docs/", v5emb.New("Waver Flags", "/docs/openapi.yaml", "/docs/"))

	dist := findDist()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ofrep/") {
			shared.WriteErr(w, 404, "not found")
			return
		}
		if dist != "" {
			p := filepath.Join(dist, filepath.Clean(strings.TrimPrefix(r.URL.Path, "/")))
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				http.ServeFile(w, r, p)
				return
			}
			http.ServeFile(w, r, filepath.Join(dist, "index.html"))
			return
		}
		shared.WriteErr(w, 404, "web dist missing, run web build")
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Admin JWT only guards /api/v1. OFREP uses API keys.
		if strings.HasPrefix(r.URL.Path, "/api/v1/") &&
			r.URL.Path != "/api/v1/auth/login" &&
			r.URL.Path != "/api/v1/auth/change-password" {
			if !iam.IsAdmin(r, cfg.JWTSecret) {
				slog.Warn("admin denied", "path", r.URL.Path, "reason", "bad jwt")
				shared.WriteErr(w, 401, "unauthorized")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
	return withLogging(inner)
}

// withLogging records one slog line per request.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path,
			"status", sw.code, "dur_ms", time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func findDist() string {
	for _, d := range []string{"/app/web/dist", "./web/dist", "../web/dist", "../../web/dist"} {
		if st, err := os.Stat(filepath.Join(d, "index.html")); err == nil && !st.IsDir() {
			return d
		}
	}
	return ""
}
