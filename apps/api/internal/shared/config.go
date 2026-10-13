package shared

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings. Env wins.
type Config struct {
	Port      string
	TCPPort   string
	DBPath    string
	JWTSecret string

	// Replication. All optional: with no peers configured this stays a single
	// node that still records clocks, so enabling peers later needs no rewrite.
	NodeID       string
	Peers        []string
	SyncSecret   string
	SyncInterval time.Duration
	CacheBytes   int
}

func LoadConfig() Config {
	loadRootEnv()
	return Config{
		Port:         getenv("PORT", "8080"),
		TCPPort:      getenv("TCP_PORT", "9090"),
		DBPath:       getenv("DB_PATH", "./data/waver.db"),
		JWTSecret:    os.Getenv("JWT_SECRET"),
		NodeID:       os.Getenv("NODE_ID"),
		Peers:        splitList(os.Getenv("PEERS")),
		SyncSecret:   os.Getenv("SYNC_SECRET"),
		SyncInterval: time.Duration(AtoiDefault(getenv("SYNC_INTERVAL_MS", "1000"), 1000)) * time.Millisecond,
		CacheBytes:   AtoiDefault(getenv("CACHE_MB", "64"), 64) << 20,
	}
}

// splitList parses a comma-separated env list, dropping blanks.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
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
