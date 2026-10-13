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
	"github.com/daffadon/waver/internal/sync"
	"github.com/daffadon/waver/internal/tcp"
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

	// The register loads every clock into memory and warms fastcache from sqlite,
	// so evaluations are served without touching the database.
	flagStore, err := flag.NewStore(db, cfg.NodeID, cfg.CacheBytes)
	if err != nil {
		slog.Error("open flag register failed", "err", err)
		os.Exit(1)
	}
	if cfg.SyncSecret == "" {
		cfg.SyncSecret = cfg.JWTSecret
	}

	iamH := &iam.Handler{Store: iamStore, Cfg: cfg}
	flagH := &flag.Handler{Store: flagStore}
	ofrepH := &ofrep.Handler{Flags: flagStore, Keys: iamStore}
	router := app.Router(cfg, iamH, flagH, ofrepH, app.NodeStatus{
		Node: flagStore.NodeID, Peers: cfg.Peers, Store: flagStore,
	})
	tcpSrv := &tcp.Server{Handler: router}

	// No peers means single node: same behaviour as before replication existed.
	// The syncer lives as long as the process, so this channel is never closed.
	stop := make(chan struct{})
	if len(cfg.Peers) > 0 {
		syncer := sync.New(flagStore, cfg.Peers, cfg.SyncSecret, cfg.SyncInterval)
		syncer.Start(stop)
		ofrepH.Sync = syncer
		tcpSrv.Sync = syncer
		slog.Info("replication on", "node", flagStore.NodeID, "peers", cfg.Peers,
			"interval", cfg.SyncInterval.String())
	}

	go func() {
		if err := tcpSrv.Serve(cfg.TCPPort); err != nil {
			slog.Error("tcp stopped", "err", err)
			os.Exit(1)
		}
	}()

	slog.Info("listening", "port", cfg.Port, "tcp", cfg.TCPPort, "db", cfg.DBPath, "node", flagStore.NodeID)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
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
