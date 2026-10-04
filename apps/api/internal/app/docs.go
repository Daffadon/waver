package app

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/daffadon/waver/internal/shared"
)

// findSpec locates the contract file in dev and container layouts.
func findSpec() string {
	for _, p := range []string{
		"/app/contracts/openapi.yaml",
		"../../packages/contracts/openapi.yaml",
		"../../../packages/contracts/openapi.yaml",
		"../../../../packages/contracts/openapi.yaml",
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			abs, err := filepath.Abs(p)
			if err == nil {
				return abs
			}
			return p
		}
	}
	return ""
}

// serveSpec serves the exact contract file clients code against.
func serveSpec(w http.ResponseWriter, r *http.Request) {
	p := findSpec()
	if p == "" {
		slog.Error("spec missing", "reason", "openapi.yaml not found")
		shared.WriteErr(w, 500, "spec missing")
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	http.ServeFile(w, r, p)
}
