package shared

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// OpenDB opens sqlite with WAL. Runs migrations/*.sql in order.
//
// Pragmas ride in the DSN so every pooled connection gets them: busy_timeout is
// per connection, and replicated writes now race admin writes for sqlite's single
// writer lock. Setting them once with Exec would cover one connection and leave
// the rest failing with SQLITE_BUSY. _txlock=immediate takes the write lock at
// Begin, so no transaction can start as a reader and fail on upgrade.
func OpenDB(dbPath, migrationsDir string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_txlock", "immediate")
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	db, err := sql.Open("sqlite", dbPath+sep+q.Encode())
	if err != nil {
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
