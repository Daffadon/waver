package flag

import (
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/daffadon/waver/internal/shared"
)

func newTestStore(t *testing.T, nodeID string) (*Store, *sql.DB) {
	t.Helper()
	db, err := shared.OpenDB(filepath.Join(t.TempDir(), "waver.db"), "../../migrations")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := NewStore(db, nodeID, 1<<20)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s, db
}

func testFlag(key, name string) Flag {
	return Flag{Key: key, Name: name, Enabled: true, Rules: []Rule{}}
}

func testRecord(key, node string, lamport int64, name string) Record {
	return Record{Key: key, Clock: Clock{Lamport: lamport, Node: node}, At: 1,
		Flag: &Flag{Key: key, Name: name, Enabled: true, Rules: []Rule{}}}
}

func tombstone(key, node string, lamport int64) Record {
	return Record{Key: key, Clock: Clock{Lamport: lamport, Node: node}, Deleted: true, At: 1}
}

func TestWriteStampsClockAndServesCache(t *testing.T) {
	s, _ := newTestStore(t, "a")

	saved, err := s.Upsert(testFlag("beta", "Beta"), 0)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if saved.Version != 1 || saved.UpdatedAt == 0 {
		t.Fatalf("first write version = %d, updated_at = %d, want 1 and now", saved.Version, saved.UpdatedAt)
	}
	saved, err = s.Upsert(testFlag("beta", "Beta v2"), 0)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if saved.Version != 2 {
		t.Fatalf("second write version = %d, want 2", saved.Version)
	}

	got, err := s.Get("beta")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Beta v2" || got.Version != 2 {
		t.Fatalf("get = %+v, want the second write", got)
	}
	if s.Cache.Len() != 1 {
		t.Fatalf("cache holds %d flags, want 1", s.Cache.Len())
	}

	if _, err := s.Upsert(testFlag("alpha", "Alpha"), 0); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Key != "alpha" || list[1].Key != "beta" {
		t.Fatalf("list = %+v, want alpha then beta", list)
	}
	if s.Revision() != 5 {
		t.Fatalf("revision = %d, want 5 (beta 2 + alpha 3)", s.Revision())
	}
}

func TestApplyKeepsTheNewerClock(t *testing.T) {
	s, _ := newTestStore(t, "a")
	if _, err := s.Upsert(testFlag("k", "mine"), 0); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	changed, err := s.Apply(testRecord("k", "b", 5, "theirs"))
	if err != nil || !changed {
		t.Fatalf("apply newer = %v, %v, want true, nil", changed, err)
	}
	got, err := s.Get("k")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "theirs" || got.Version != 5 {
		t.Fatalf("get = %+v, want the peer value at version 5", got)
	}

	if changed, err = s.Apply(testRecord("k", "b", 5, "theirs")); err != nil || changed {
		t.Fatalf("replay = %v, %v, want false, nil", changed, err)
	}
	if changed, err = s.Apply(testRecord("k", "c", 2, "stale")); err != nil || changed {
		t.Fatalf("stale = %v, %v, want false, nil", changed, err)
	}
	// A write after learning a remote clock still outranks it.
	saved, err := s.Upsert(testFlag("k", "mine again"), 0)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if saved.Version != 6 {
		t.Fatalf("local write after remote clock = %d, want 6", saved.Version)
	}
}

func TestTombstoneBlocksResurrection(t *testing.T) {
	s, _ := newTestStore(t, "a")
	if _, err := s.Upsert(testFlag("k", "live"), 0); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if changed, err := s.Apply(tombstone("k", "b", 7)); err != nil || !changed {
		t.Fatalf("apply delete = %v, %v, want true, nil", changed, err)
	}
	if _, err := s.Get("k"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get after delete = %v, want ErrNoRows", err)
	}
	if s.Cache.Len() != 0 {
		t.Fatalf("cache holds %d flags after delete, want 0", s.Cache.Len())
	}
	// The node that was offline during the delete still holds the old value.
	if changed, err := s.Apply(testRecord("k", "c", 3, "zombie")); err != nil || changed {
		t.Fatalf("resurrect = %v, %v, want false, nil", changed, err)
	}
	if _, err := s.Get("k"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get after resurrect attempt = %v, want ErrNoRows", err)
	}
}

func TestEqualClocksBreakOnNode(t *testing.T) {
	a, _ := newTestStore(t, "a")
	b, _ := newTestStore(t, "b")
	low := testRecord("k", "aaa", 9, "low")
	high := testRecord("k", "bbb", 9, "high")

	for _, s := range []*Store{a, b} {
		if _, err := s.Apply(low); err != nil {
			t.Fatalf("apply low: %v", err)
		}
		if _, err := s.Apply(high); err != nil {
			t.Fatalf("apply high: %v", err)
		}
	}
	for _, s := range []*Store{a, b} {
		got, err := s.Get("k")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Name != "high" {
			t.Fatalf("winner = %q, want the higher node id", got.Name)
		}
	}
	da, ra := a.Digest()
	db, rb := b.Digest()
	if da != db || ra != rb {
		t.Fatalf("digests %d/%d revisions %d/%d, want equal", da, db, ra, rb)
	}
}

func TestDigestIgnoresArrivalOrder(t *testing.T) {
	a, _ := newTestStore(t, "a")
	b, _ := newTestStore(t, "b")
	recs := []Record{
		testRecord("one", "x", 1, "one"),
		testRecord("two", "y", 4, "two"),
		tombstone("three", "x", 9),
	}
	for _, r := range recs {
		if _, err := a.Apply(r); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	for i := len(recs) - 1; i >= 0; i-- {
		if _, err := b.Apply(recs[i]); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	da, ra := a.Digest()
	db, rb := b.Digest()
	if da != db || ra != rb {
		t.Fatalf("digests %d/%d revisions %d/%d, want equal", da, db, ra, rb)
	}
	// Any further write must move the digest, or peers would stop comparing.
	if _, err := a.Apply(testRecord("one", "x", 2, "one again")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if d, _ := a.Digest(); d == da {
		t.Fatal("digest unchanged after a newer record")
	}
}

func TestConflictOnStaleBase(t *testing.T) {
	s, _ := newTestStore(t, "a")
	if _, err := s.Upsert(testFlag("k", "v1"), 0); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	saved, err := s.Upsert(testFlag("k", "v2"), 1)
	if err != nil {
		t.Fatalf("upsert with matching base: %v", err)
	}
	if saved.Version != 2 {
		t.Fatalf("version = %d, want 2", saved.Version)
	}
	if _, err := s.Upsert(testFlag("k", "v3"), 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("upsert with stale base = %v, want ErrConflict", err)
	}
	if _, err := s.Upsert(testFlag("k", "v3"), 0); err != nil {
		t.Fatalf("upsert without base: %v", err)
	}
	// A remote write makes the caller's base stale too.
	if _, err := s.Apply(testRecord("k", "b", 9, "remote")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := s.Upsert(testFlag("k", "v4"), 3); !errors.Is(err, ErrConflict) {
		t.Fatalf("upsert after remote write = %v, want ErrConflict", err)
	}
}

func TestBootWarmsCacheFromDB(t *testing.T) {
	s, db := newTestStore(t, "a")
	if _, err := s.Upsert(testFlag("k1", "one"), 0); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := s.Upsert(testFlag("k2", "two"), 0); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// A restart builds memory and cache from sqlite alone.
	restarted, err := NewStore(db, "a", 1<<20)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if restarted.Cache.Len() != 2 {
		t.Fatalf("warmed cache holds %d flags, want 2", restarted.Cache.Len())
	}
	if restarted.Revision() != 3 {
		t.Fatalf("revision after restart = %d, want 3", restarted.Revision())
	}

	// Reads are served from the cache: removing the rows behind its back does not
	// change the answer.
	if _, err := db.Exec(`DELETE FROM flags`); err != nil {
		t.Fatalf("delete rows: %v", err)
	}
	got, err := restarted.Get("k1")
	if err != nil {
		t.Fatalf("get after out-of-band delete: %v", err)
	}
	if got.Name != "one" {
		t.Fatalf("get = %+v, want the cached flag", got)
	}
}

func TestBackfillsClocksForPreMigrationFlags(t *testing.T) {
	_, db := newTestStore(t, "a")
	// A database written before replication existed: flags rows, no clocks.
	if _, err := db.Exec(`INSERT INTO flags(key,name,enabled,default_value,rules,version,updated_at)
		VALUES('old','Old',1,0,'[]',4,123)`); err != nil {
		t.Fatalf("insert legacy flag: %v", err)
	}
	upgraded, err := NewStore(db, "a", 1<<20)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if upgraded.Revision() != 4 {
		t.Fatalf("revision = %d, want the legacy version 4", upgraded.Revision())
	}
	got, err := upgraded.Get("old")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Version != 4 {
		t.Fatalf("version = %d, want 4", got.Version)
	}
	recs := upgraded.Records()
	if len(recs) != 1 || recs[0].Clock.Node != "a" || recs[0].Clock.Lamport != 4 {
		t.Fatalf("records = %+v, want one record clocked 4/a", recs)
	}
}

func TestDeleteSurvivesRestart(t *testing.T) {
	s, db := newTestStore(t, "a")
	if _, err := s.Upsert(testFlag("k", "live"), 0); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.Delete("k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	restarted, err := NewStore(db, "a", 1<<20)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := restarted.Get("k"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get = %v, want ErrNoRows", err)
	}
	if restarted.Cache.Len() != 0 {
		t.Fatalf("cache holds %d flags, want 0", restarted.Cache.Len())
	}
	if changed, err := restarted.Apply(testRecord("k", "b", 1, "zombie")); err != nil || changed {
		t.Fatalf("resurrect after restart = %v, %v, want false, nil", changed, err)
	}
}

func TestCacheIsConcurrencySafe(t *testing.T) {
	c := NewCache(1 << 20)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + i))
			for j := range 50 {
				f := testFlag(key, "n")
				f.Version = int64(j)
				c.Put(f)
				if _, ok := c.Get(key); !ok {
					t.Errorf("get %s missed after put", key)
				}
				_ = c.Keys()
				if j%3 == 0 {
					c.Del(key)
				}
			}
		}(i)
	}
	wg.Wait()
	if c.Len() != len(c.Keys()) {
		t.Fatalf("len %d disagrees with keys %d", c.Len(), len(c.Keys()))
	}
}

func TestRecordsNewerThanReturnsOnlyTheDiff(t *testing.T) {
	s, _ := newTestStore(t, "a")
	for _, k := range []string{"k1", "k2", "k3"} {
		if _, err := s.Upsert(testFlag(k, k), 0); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	peer := map[string]Clock{}
	for _, r := range s.Records() {
		if r.Key != "k3" {
			peer[r.Key] = r.Clock
		}
	}
	diff := s.RecordsNewerThan(peer)
	if len(diff) != 1 || diff[0].Key != "k3" {
		t.Fatalf("diff = %+v, want only k3", diff)
	}
}
