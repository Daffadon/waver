package shared

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// OpenDB opens sqlite with WAL. Runs migrations/*.sql in order.
func OpenDB(dbPath, migrationsDir string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(migrationsDir, e.Name()))
		if err != nil {
			return nil, err
		}
		if _, err := db.Exec(string(b)); err != nil {
			return nil, err
		}
	}
	return db, nil
}
