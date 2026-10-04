package shared

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds runtime settings. Env wins.
type Config struct {
	Port      string
	DBPath    string
	JWTSecret string
}

func LoadConfig() Config {
	loadRootEnv()
	port := getenv("PORT", "8080")
	db := getenv("DB_PATH", "./data/waver.db")
	secret := os.Getenv("JWT_SECRET")
	return Config{Port: port, DBPath: db, JWTSecret: secret}
}

// loadRootEnv reads KEY=VALUE lines from the repo-root .env.
// Skips blanks and # comments. Never overwrites real env.
func loadRootEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for i := 0; i < 6; i++ {
		p := filepath.Join(dir, ".env")
		if b, err := os.ReadFile(p); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				line = strings.TrimPrefix(line, "export ")
				k, v, ok := strings.Cut(line, "=")
				k = strings.TrimSpace(k)
				if !ok || k == "" || os.Getenv(k) != "" {
					continue
				}
				v = strings.TrimSpace(v)
				v = strings.Trim(v, `"'`)
				_ = os.Setenv(k, v)
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteErr(w http.ResponseWriter, code int, msg string) {
	WriteJSON(w, code, map[string]string{"error": msg})
}

func AtoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
