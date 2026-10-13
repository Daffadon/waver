package sync_test

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/daffadon/waver/internal/flag"
	"github.com/daffadon/waver/internal/shared"
	"github.com/daffadon/waver/internal/sync"
	"github.com/daffadon/waver/internal/tcp"
)

// node is one Waver process: its register, its TCP port, and (once wired) its
// syncer. The tests drive real sockets rather than the merge functions alone.
type node struct {
	store  *flag.Store
	syncer *sync.Syncer
	srv    *tcp.Server
	ln     net.Listener
	addr   string
	stop   chan struct{}
}

func newNode(t *testing.T) *node {
	t.Helper()
	db, err := shared.OpenDB(filepath.Join(t.TempDir(), "waver.db"), "../../migrations")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := flag.NewStore(db, "", 1<<20)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	n := &node{
		store: store,
		srv:   &tcp.Server{Handler: http.NotFoundHandler()},
		ln:    ln,
		addr:  ln.Addr().String(),
		stop:  make(chan struct{}),
	}
	t.Cleanup(func() {
		close(n.stop)
		_ = ln.Close()
	})
	return n
}

// connect installs a syncer for the given peers and starts serving the port.
// Sync is set before the listener accepts anything, so nothing races.
func (n *node) connect(t *testing.T, secret string, interval time.Duration, peers ...string) {
	t.Helper()
	n.syncer = sync.New(n.store, peers, secret, interval)
	n.srv.Sync = n.syncer
	n.syncer.Start(n.stop)
	go func() { _ = n.srv.ServeOn(n.ln) }()
}

// wire builds the full mesh the design assumes: every node knows every other.
func wire(t *testing.T, secret string, nodes ...*node) {
	t.Helper()
	for _, n := range nodes {
		var peers []string
		for _, o := range nodes {
			if o != n {
				peers = append(peers, o.addr)
			}
		}
		n.connect(t, secret, 50*time.Millisecond, peers...)
	}
}

func write(t *testing.T, n *node, key, name string) {
	t.Helper()
	if _, err := n.store.Upsert(flag.Flag{Key: key, Name: name, Enabled: true, Rules: []flag.Rule{}}, 0); err != nil {
		t.Fatalf("upsert %s: %v", key, err)
	}
}

// waitSame blocks until every node reports the same digest and revision, which
// means they hold the same keys with the same clocks.
func waitSame(t *testing.T, nodes ...*node) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		d0, r0 := nodes[0].store.Digest()
		same, state := true, fmt.Sprintf("%s: digest=%d rev=%d", nodes[0].store.NodeID, d0, r0)
		for _, n := range nodes[1:] {
			d, r := n.store.Digest()
			state += fmt.Sprintf(" | %s: digest=%d rev=%d", n.store.NodeID, d, r)
			if d != d0 || r != r0 {
				same = false
			}
		}
		if same {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("nodes did not converge: %s", state)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func get(t *testing.T, n *node, key string) flag.Flag {
	t.Helper()
	f, err := n.store.Get(key)
	if err != nil {
		t.Fatalf("%s: get %s: %v", n.store.NodeID, key, err)
	}
	return f
}

func TestThreeNodesConverge(t *testing.T) {
	n1, n2, n3 := newNode(t), newNode(t), newNode(t)
	wire(t, "s3cret", n1, n2, n3)

	write(t, n1, "checkout", "Checkout")
	waitSame(t, n1, n2, n3)
	for _, n := range []*node{n1, n2, n3} {
		if got := get(t, n, "checkout"); got.Name != "Checkout" || got.Version != 1 {
			t.Fatalf("%s: got %+v, want Checkout at version 1", n.store.NodeID, got)
		}
	}

	// A write on a node that did not originate the flag comes back the same way.
	write(t, n2, "checkout", "Checkout v2")
	waitSame(t, n1, n2, n3)
	for _, n := range []*node{n1, n2, n3} {
		if got := get(t, n, "checkout"); got.Name != "Checkout v2" || got.Version != 2 {
			t.Fatalf("%s: got %+v, want Checkout v2 at version 2", n.store.NodeID, got)
		}
	}
}

func TestDeleteReplicates(t *testing.T) {
	n1, n2 := newNode(t), newNode(t)
	wire(t, "s3cret", n1, n2)

	write(t, n1, "checkout", "Checkout")
	waitSame(t, n1, n2)
	if err := n2.store.Delete("checkout"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	waitSame(t, n1, n2)
	for _, n := range []*node{n1, n2} {
		if _, err := n.store.Get("checkout"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("%s: get after delete = %v, want ErrNoRows", n.store.NodeID, err)
		}
	}
}

func TestOfflineNodeCatchesUp(t *testing.T) {
	n1, n2, n3 := newNode(t), newNode(t), newNode(t)
	wire(t, "s3cret", n1, n2)
	write(t, n1, "checkout", "Checkout")
	waitSame(t, n1, n2)

	// n3 has been off the whole time. The digest round hands it the full state.
	if _, err := n3.store.Get("checkout"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("offline node already has the flag: %v", err)
	}
	n3.connect(t, "s3cret", 50*time.Millisecond, n1.addr, n2.addr)
	waitSame(t, n1, n2, n3)
	if got := get(t, n3, "checkout"); got.Name != "Checkout" {
		t.Fatalf("caught-up node got %+v", got)
	}
}

func TestStaleNodeCannotResurrect(t *testing.T) {
	n1, n2, n3 := newNode(t), newNode(t), newNode(t)
	// n3 edits the flag, then goes offline before the others ever hear about it.
	write(t, n3, "checkout", "Offline edit")
	wire(t, "s3cret", n1, n2)
	write(t, n1, "checkout", "Checkout")
	write(t, n1, "checkout", "Checkout v2")
	if err := n1.store.Delete("checkout"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	waitSame(t, n1, n2)

	n3.connect(t, "s3cret", 50*time.Millisecond, n1.addr, n2.addr)
	waitSame(t, n1, n2, n3)
	for _, n := range []*node{n1, n2, n3} {
		if _, err := n.store.Get("checkout"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("%s: stale edit won, get = %v", n.store.NodeID, err)
		}
	}
}

func TestFencePullsBeforeAnswering(t *testing.T) {
	n1, n2, n3 := newNode(t), newNode(t), newNode(t)
	wire(t, "s3cret", n1, n2)
	write(t, n1, "checkout", "Checkout")
	waitSame(t, n1, n2)

	// A syncer whose loop never ticks on its own: only the fence can pull.
	n3.connect(t, "s3cret", time.Hour, n1.addr, n2.addr)
	if n3.store.Revision() != 0 {
		t.Fatalf("n3 revision = %d, want 0 before the fence", n3.store.Revision())
	}
	want := n1.store.Revision()
	if !n3.syncer.CatchUp(want, 2*time.Second) {
		t.Fatalf("catch-up failed to reach revision %d, have %d", want, n3.store.Revision())
	}
	if got := get(t, n3, "checkout"); got.Name != "Checkout" {
		t.Fatalf("fenced read got %+v", got)
	}
	// A revision nobody has ever reached times out instead of blocking forever.
	start := time.Now()
	if n3.syncer.CatchUp(want+1000, 100*time.Millisecond) {
		t.Fatal("catch-up claimed a revision no node holds")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("catch-up waited %v, want it bounded", elapsed)
	}
}

func TestUnauthenticatedFramesAreRefused(t *testing.T) {
	n1 := newNode(t)
	wire(t, "s3cret", n1)

	body := `{"type":"sync_state","node":"evil","records":[{"key":"backdoor","clock":{"lamport":99,"node":"evil"},` +
		`"flag":{"key":"backdoor","name":"Backdoor","enabled":true,"rules":[]}}],"mac":"deadbeef"}`
	reply := sendRaw(t, n1.addr, body)
	if reply["error"] == nil {
		t.Fatalf("bad mac accepted: %+v", reply)
	}
	if _, err := n1.store.Get("backdoor"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("injected flag landed: %v", err)
	}

	// Same for a digest frame, which is how an attacker would drive catch-up.
	reply = sendRaw(t, n1.addr, `{"type":"sync_digest","node":"evil","digest":1}`)
	if reply["error"] == nil {
		t.Fatalf("unsigned digest accepted: %+v", reply)
	}
}

func sendRaw(t *testing.T, addr, line string) map[string]any {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer c.Close()
	if err := c.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("deadline: %v", err)
	}
	if _, err := c.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	raw, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read reply: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("reply %q: %v", raw, err)
	}
	return out
}
