package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/daffadon/waver/internal/app"
	"github.com/daffadon/waver/internal/flag"
	"github.com/daffadon/waver/internal/iam"
	"github.com/daffadon/waver/internal/ofrep"
	"github.com/daffadon/waver/internal/shared"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	cfg := shared.LoadConfig()
	if cfg.JWTSecret == "" {
		// ponytail: dev-only fallback, set JWT_SECRET in prod
		cfg.JWTSecret = "dev-secret-change-me"
		slog.Warn("JWT_SECRET empty, using dev secret")
	}
	db, err := shared.OpenDB(cfg.DBPath, "./migrations")
	if err != nil {
		db, err = shared.OpenDB(cfg.DBPath, "/app/migrations")
		if err != nil {
			slog.Error("open db failed", "err", err)
			os.Exit(1)
		}
	}

	iamStore := &iam.Store{DB: db}
	seedAdmin(iamStore)

	iamH := &iam.Handler{Store: iamStore, Cfg: cfg}
	flagH := &flag.Handler{Store: &flag.Store{DB: db}}
	ofrepH := &ofrep.Handler{Flags: &flag.Store{DB: db}, Keys: iamStore}

	slog.Info("listening", "port", cfg.Port, "db", cfg.DBPath)
	if err := http.ListenAndServe(":"+cfg.Port, app.Router(cfg, iamH, flagH, ofrepH)); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// seedAdmin creates single admin once. Prints generated password once.
func seedAdmin(s *iam.Store) {
	_, found, err := s.GetAdmin()
	if err != nil {
		slog.Error("seed admin failed", "err", err)
		os.Exit(1)
	}
	if found {
		return
	}
	pw, err := iam.RandomPassword()
	if err != nil {
		slog.Error("seed admin failed", "err", err)
		os.Exit(1)
	}
	hash, err := iam.HashPassword(pw)
	if err != nil {
		slog.Error("seed admin failed", "err", err)
		os.Exit(1)
	}
	if err := s.PutAdmin(hash, true); err != nil {
		slog.Error("seed admin failed", "err", err)
		os.Exit(1)
	}
	slog.Info("admin seeded, password change required at first login")
	fmt.Fprintln(os.Stderr, "========================================")
	fmt.Fprintln(os.Stderr, "FIRST BOOT admin password (change at login):")
	fmt.Fprintln(os.Stderr, "  username: admin")
	fmt.Fprintln(os.Stderr, "  password: "+pw)
	fmt.Fprintln(os.Stderr, "========================================")
}
