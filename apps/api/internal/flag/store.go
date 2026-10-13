package flag

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ErrConflict reports a save against a stale base revision: something wrote the
// flag since the caller loaded it, here or on another node.
var ErrConflict = errors.New("conflict: flag changed on another node, reload and retry")

// keyState is the merge metadata kept in memory for every key this node has ever
// seen, live or deleted. The value itself lives in the cache.
type keyState struct {
	Clock   Clock
	Deleted bool
	At      int64
}

// Store is the flag register: the whole flag map, a per-key Lamport clock, and
// fastcache in front of sqlite.
//
// A local write and a replicated write go through the same apply path, so this
// node can never treat its own writes differently from a peer's.
//
// Locking: mu guards clocks, rev, digest and the sqlite writes that keep them
// honest. Reads hit the cache without touching mu and only take mu.RLock on a
// miss, so a stale refill can never land after a newer write.
type Store struct {
	DB     *sql.DB
	Cache  *Cache
	NodeID string

	mu          sync.RWMutex
	clocks      map[string]keyState
	rev         int64
	digest      uint64
	nodeLamport int64

	// onLocal announces a locally written record to peers. Set once, before the
	// store serves traffic.
	onLocal func(Record)
}

// NewStore opens the register: it adopts or claims this node's identity,
// backfills clocks for flags written before replication existed, loads every
// clock into memory and warms the cache from sqlite.
func NewStore(db *sql.DB, nodeID string, cacheBytes int) (*Store, error) {
	s := &Store{DB: db, Cache: NewCache(cacheBytes), clocks: make(map[string]keyState)}
	if err := s.load(nodeID); err != nil {
		return nil, err
	}
	return s, nil
}

// OnLocal installs the announce hook for locally written records.
func (s *Store) OnLocal(fn func(Record)) { s.onLocal = fn }

// Get serves the cache first. A miss re-reads sqlite under the register lock, so
// a concurrent apply can never leave an older value cached behind a newer one.
func (s *Store) Get(key string) (Flag, error) {
	if f, ok := s.Cache.Get(key); ok {
		return f, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if st, ok := s.clocks[key]; !ok || st.Deleted {
		return Flag{}, sql.ErrNoRows
	}
	if f, ok := s.Cache.Get(key); ok {
		return f, nil
	}
	f, err := s.getDB(key)
	if err != nil {
		return Flag{}, err
	}
	s.Cache.Put(f)
	return f, nil
}

// List walks the cached key index in key order, refilling from sqlite if a key
// was evicted. A flag deleted mid-list is skipped rather than failing the call.
func (s *Store) List() ([]Flag, error) {
	keys := s.Cache.Keys()
	out := make([]Flag, 0, len(keys))
	for _, k := range keys {
		f, err := s.Get(k)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// Len is the number of live flags this node holds.
func (s *Store) Len() int { return s.Cache.Len() }

// Upsert writes a flag locally: it stamps a fresh clock and runs the same apply
// path a peer's record takes. base > 0 requires the key to sit at exactly that
// revision, which is how a caller detects an edit it did not make.
func (s *Store) Upsert(f Flag, base int64) (Flag, error) {
	return s.write(f.Key, &f, false, base)
}

// Delete tombstones a flag locally. The tombstone replicates like a value, so a
// node that was offline during the delete cannot bring the flag back.
func (s *Store) Delete(key string) error {
	_, err := s.write(key, nil, true, 0)
	return err
}

// Apply merges one replicated record, reporting whether it changed anything.
// Stale and equal clocks are ignored, so replays are free and a node back from a
// week offline cannot undo a newer change.
func (s *Store) Apply(rec Record) (bool, error) {
	if err := validateRecord(rec); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !rec.Clock.After(s.clocks[rec.Key].Clock) {
		return false, nil
	}
	// Learn the remote clock so a later local write still outranks it.
	if rec.Clock.Lamport > s.nodeLamport {
		s.nodeLamport = rec.Clock.Lamport
	}
	if err := s.applyLocked(rec, false); err != nil {
		return false, err
	}
	return true, nil
}

// Records snapshots the whole replicated state, tombstones included.
func (s *Store) Records() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.recordsLocked(nil)
}

// RecordsNewerThan returns the records that beat the peer's clocks: the reply
// half of a push, so only the diff travels back.
func (s *Store) RecordsNewerThan(peer map[string]Clock) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.recordsLocked(peer)
}

// Digest returns the state fingerprint and the node revision in one snapshot.
// Equal digests mean both nodes hold the same keys with the same clocks and
// tombstones, so no payload has to move.
func (s *Store) Digest() (uint64, int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.digest, s.rev
}

// Revision is this node's opaque revision: the sum of every key clock. It grows
// with any accepted write and converged nodes report the same number, which is
// what makes it usable as an X-Waver-Min-Rev fence.
func (s *Store) Revision() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rev
}

// write stamps a fresh clock and applies it. The caller decides whether the
// record is a value or a tombstone.
func (s *Store) write(key string, f *Flag, deleted bool, base int64) (Flag, error) {
	if err := ValidateKey(key); err != nil {
		return Flag{}, err
	}
	s.mu.Lock()
	cur := s.clocks[key]
	if base > 0 && cur.Clock.Lamport != base {
		s.mu.Unlock()
		return Flag{}, ErrConflict
	}
	now := time.Now().UTC().Unix()
	lamport := max(s.nodeLamport, cur.Clock.Lamport) + 1
	s.nodeLamport = lamport
	rec := Record{Key: key, Clock: Clock{Lamport: lamport, Node: s.NodeID}, Deleted: deleted, At: now}
	if !deleted {
		v := *f
		v.Key = key
		v.Version = lamport
		v.UpdatedAt = now
		if v.Rules == nil {
			v.Rules = []Rule{}
		}
		rec.Flag = &v
	}
	err := s.applyLocked(rec, true)
	s.mu.Unlock()
	if err != nil {
		return Flag{}, err
	}
	if s.onLocal != nil {
		s.onLocal(rec)
	}
	if rec.Flag == nil {
		return Flag{}, nil
	}
	return *rec.Flag, nil
}

// applyLocked persists rec and moves memory and cache to match. The caller holds
// s.mu and has already decided rec wins; a losing record never gets here.
func (s *Store) applyLocked(rec Record, local bool) error {
	if !rec.Deleted {
		// A peer's payload must not steer our cache index, and the clock is the
		// revision whatever the payload claims.
		rec.Flag.Key = rec.Key
		rec.Flag.Version = rec.Clock.Lamport
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if rec.Deleted {
		if _, err := tx.Exec(`DELETE FROM flags WHERE key=?`, rec.Key); err != nil {
			return err
		}
	} else {
		raw, err := json.Marshal(rec.Flag.Rules)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO flags(key,name,description,enabled,default_value,rules,starts_at,ends_at,version,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(key) DO UPDATE SET name=excluded.name, description=excluded.description, enabled=excluded.enabled,
			default_value=excluded.default_value, rules=excluded.rules, starts_at=excluded.starts_at, ends_at=excluded.ends_at,
			version=excluded.version, updated_at=excluded.updated_at`,
			rec.Key, rec.Flag.Name, rec.Flag.Description, boolToInt(rec.Flag.Enabled), boolToInt(rec.Flag.DefaultValue),
			string(raw), rec.Flag.StartsAt, rec.Flag.EndsAt, rec.Clock.Lamport, rec.At); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO key_clock(key,lamport,node_id,deleted,at) VALUES(?,?,?,?,?)
		ON CONFLICT(key) DO UPDATE SET lamport=excluded.lamport, node_id=excluded.node_id, deleted=excluded.deleted, at=excluded.at`,
		rec.Key, rec.Clock.Lamport, rec.Clock.Node, boolToInt(rec.Deleted), rec.At); err != nil {
		return err
	}
	if local {
		if _, err := tx.Exec(`UPDATE node_clock SET lamport=? WHERE id=1`, s.nodeLamport); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// sqlite is the source of truth, so only now move memory and cache.
	old := s.clocks[rec.Key]
	s.clocks[rec.Key] = keyState{Clock: rec.Clock, Deleted: rec.Deleted, At: rec.At}
	s.rev += rec.Clock.Lamport - old.Clock.Lamport
	s.digest += clockDigest(rec.Key, rec.Clock, rec.Deleted) - clockDigest(rec.Key, old.Clock, old.Deleted)
	if rec.Deleted {
		s.Cache.Del(rec.Key)
	} else {
		s.Cache.Put(*rec.Flag)
	}
	return nil
}

// recordsLocked snapshots the register. With a peer map, only records that beat
// that peer travel.
func (s *Store) recordsLocked(peer map[string]Clock) []Record {
	out := make([]Record, 0, len(s.clocks))
	for key, st := range s.clocks {
		if peer != nil {
			if c, ok := peer[key]; ok && !st.Clock.After(c) {
				continue
			}
		}
		rec := Record{Key: key, Clock: st.Clock, Deleted: st.Deleted, At: st.At}
		if !st.Deleted {
			f, ok := s.Cache.Get(key)
			if !ok {
				var err error
				f, err = s.getDB(key)
				if err != nil {
					slog.Warn("record without value skipped", "key", key, "err", err)
					continue
				}
				s.Cache.Put(f)
			}
			rec.Flag = &f
		}
		out = append(out, rec)
	}
	return out
}

// validateRecord rejects a peer record that would corrupt the register. Peers
// are authenticated, but this is a trust boundary and the checks are cheap.
func validateRecord(rec Record) error {
	if err := ValidateKey(rec.Key); err != nil {
		return err
	}
	if rec.Clock.Node == "" || rec.Clock.Lamport <= 0 {
		return fmt.Errorf("record %s: bad clock", rec.Key)
	}
	if rec.Deleted {
		return nil
	}
	if rec.Flag == nil {
		return fmt.Errorf("record %s: live record without a flag", rec.Key)
	}
	if rec.Flag.Name == "" {
		return fmt.Errorf("record %s: name required", rec.Key)
	}
	return ValidateRules(rec.Flag.Rules)
}

// load adopts the node identity, backfills clocks, fills memory and warms the
// cache. Everything it needs comes from sqlite, so a restart rebuilds the exact
// state a peer would otherwise have to push.
func (s *Store) load(nodeID string) error {
	var stored string
	var lamport int64
	if err := s.DB.QueryRow(`SELECT node_id, lamport FROM node_clock WHERE id=1`).Scan(&stored, &lamport); err != nil {
		return fmt.Errorf("read node_clock: %w", err)
	}
	if nodeID == "" {
		nodeID = stored
	}
	if nodeID == "" {
		id, err := randomNodeID()
		if err != nil {
			return err
		}
		nodeID = id
		slog.Info("node id generated and stored, set NODE_ID to pin it", "node", nodeID)
	}
	if nodeID != stored {
		if _, err := s.DB.Exec(`UPDATE node_clock SET node_id=? WHERE id=1`, nodeID); err != nil {
			return fmt.Errorf("claim node id: %w", err)
		}
	}
	s.NodeID = nodeID
	s.nodeLamport = lamport

	// Flags written before this migration have no clock: seed them from their
	// version so an upgrade keeps its history and peers can still order it.
	if _, err := s.DB.Exec(`INSERT OR IGNORE INTO key_clock(key,lamport,node_id,deleted,at)
		SELECT key, MAX(version,1), ?, 0, updated_at FROM flags`, nodeID); err != nil {
		return fmt.Errorf("backfill key_clock: %w", err)
	}
	if err := s.loadClocks(); err != nil {
		return err
	}
	flags, err := s.listDB()
	if err != nil {
		return err
	}
	for _, f := range flags {
		s.Cache.Put(f)
	}
	s.nodeLamport = max(s.nodeLamport, s.maxClock())

	// Repair: a clock that claims live but has no value would replicate forever
	// without a payload.
	for key, st := range s.clocks {
		if st.Deleted {
			continue
		}
		if _, ok := s.Cache.Get(key); ok {
			continue
		}
		slog.Warn("live clock without a flag, marking deleted", "key", key)
		s.digest += clockDigest(key, st.Clock, true) - clockDigest(key, st.Clock, false)
		st.Deleted = true
		s.clocks[key] = st
		if _, err := s.DB.Exec(`UPDATE key_clock SET deleted=1 WHERE key=?`, key); err != nil {
			return fmt.Errorf("repair %s: %w", key, err)
		}
	}
	slog.Info("register loaded", "node", s.NodeID, "keys", len(s.clocks),
		"flags", s.Cache.Len(), "revision", s.rev)
	return nil
}

func (s *Store) loadClocks() error {
	rows, err := s.DB.Query(`SELECT key,lamport,node_id,deleted,at FROM key_clock`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key, node string
		var l, at int64
		var del int
		if err := rows.Scan(&key, &l, &node, &del, &at); err != nil {
			return err
		}
		st := keyState{Clock: Clock{Lamport: l, Node: node}, Deleted: del == 1, At: at}
		s.clocks[key] = st
		s.rev += l
		s.digest += clockDigest(key, st.Clock, st.Deleted)
	}
	return rows.Err()
}

func (s *Store) maxClock() int64 {
	var m int64
	for _, st := range s.clocks {
		if st.Clock.Lamport > m {
			m = st.Clock.Lamport
		}
	}
	return m
}

// randomNodeID mints an identity for a node that does not set NODE_ID. It is
// stored in node_clock, so it survives restarts.
func randomNodeID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("node id: %w", err)
	}
	return "node-" + hex.EncodeToString(b[:]), nil
}

func (s *Store) listDB() ([]Flag, error) {
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

func (s *Store) getDB(key string) (Flag, error) {
	row := s.DB.QueryRow(`SELECT key,name,description,enabled,default_value,rules,starts_at,ends_at,version,updated_at FROM flags WHERE key=?`, key)
	return scanFlag(row)
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
