package flag

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Store is the infrastructure layer for sqlite.
type Store struct{ DB *sql.DB }

func (s *Store) List() ([]Flag, error) {
	rows, err := s.DB.Query(`SELECT key,name,description,enabled,default_value,rules,starts_at,ends_at,version,updated_at FROM flags ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Flag
	for rows.Next() {
		f, err := scanFlag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) Get(key string) (Flag, error) {
	row := s.DB.QueryRow(`SELECT key,name,description,enabled,default_value,rules,starts_at,ends_at,version,updated_at FROM flags WHERE key=?`, key)
	return scanFlag(row)
}

func (s *Store) Upsert(f Flag) error {
	raw, err := json.Marshal(f.Rules)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Unix()
	_, err = s.DB.Exec(`INSERT INTO flags(key,name,description,enabled,default_value,rules,starts_at,ends_at,version,updated_at)
		VALUES(?,?,?,?,?,?,?,?,1,?)
		ON CONFLICT(key) DO UPDATE SET name=excluded.name, description=excluded.description, enabled=excluded.enabled,
		default_value=excluded.default_value, rules=excluded.rules, starts_at=excluded.starts_at, ends_at=excluded.ends_at,
		version=version+1, updated_at=excluded.updated_at`,
		f.Key, f.Name, f.Description, boolToInt(f.Enabled), boolToInt(f.DefaultValue), string(raw), f.StartsAt, f.EndsAt, now)
	return err
}

func (s *Store) Delete(key string) error {
	_, err := s.DB.Exec(`DELETE FROM flags WHERE key=?`, key)
	return err
}

type rowScanner interface{ Scan(dest ...any) error }

func scanFlag(r rowScanner) (Flag, error) {
	var f Flag
	var en, dv int
	var raw string
	var starts, ends sql.NullInt64
	err := r.Scan(&f.Key, &f.Name, &f.Description, &en, &dv, &raw, &starts, &ends, &f.Version, &f.UpdatedAt)
	if err != nil {
		return Flag{}, err
	}
	f.Enabled = en == 1
	f.DefaultValue = dv == 1
	if err := json.Unmarshal([]byte(raw), &f.Rules); err != nil {
		return Flag{}, err
	}
	if f.Rules == nil {
		f.Rules = []Rule{}
	}
	if starts.Valid {
		v := starts.Int64
		f.StartsAt = &v
	}
	if ends.Valid {
		v := ends.Int64
		f.EndsAt = &v
	}
	return f, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
