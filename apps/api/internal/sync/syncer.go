package sync

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/daffadon/waver/internal/flag"
)

// Syncer keeps this node in step with its peers: an anti-entropy loop for nodes
// that missed something, an immediate announce for local writes, and a catch-up
// pull for callers that ask for a minimum revision.
type Syncer struct {
	NodeID   string
	Peers    []string
	Interval time.Duration

	store   *flag.Store
	secret  []byte
	timeout time.Duration
	catch   chan struct{}
}

// New builds the syncer for one node. interval <= 0 means one round per second.
func New(store *flag.Store, peers []string, secret string, interval time.Duration) *Syncer {
	if interval <= 0 {
		interval = time.Second
	}
	return &Syncer{
		NodeID:   store.NodeID,
		Peers:    peers,
		Interval: interval,
		store:    store,
		secret:   []byte(secret),
		timeout:  2 * time.Second,
		catch:    make(chan struct{}, 1),
	}
}

// Start installs the announce hook and runs the anti-entropy loop until stop
// closes. Call it once, before the node serves traffic.
func (s *Syncer) Start(stop <-chan struct{}) {
	s.store.OnLocal(s.Announce)
	go s.loop(stop)
}

// loop compares digests with every peer on a tick. That is what heals a peer
// that missed an announce or was switched off.
func (s *Syncer) loop(stop <-chan struct{}) {
	t := time.NewTicker(s.Interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.round()
		}
	}
}

// round syncs with every peer concurrently and waits for all of them, so a slow
// peer delays the next tick instead of stacking rounds.
func (s *Syncer) round() {
	done := make(chan struct{}, len(s.Peers))
	for _, p := range s.Peers {
		go func(p string) {
			defer func() { done <- struct{}{} }()
			s.syncPeer(p)
		}(p)
	}
	for range s.Peers {
		<-done
	}
}

// syncPeer compares digests with one peer and pushes state when they differ.
func (s *Syncer) syncPeer(addr string) {
	digest, _ := s.store.Digest()
	rep, err := s.call(addr, s.reply(envelope{Type: typeDigest, Node: s.NodeID, Digest: digest}), typeDigestReply)
	if err != nil {
		slog.Debug("peer unreachable", "peer", addr, "err", err)
		return
	}
	if rep.Equal {
		return
	}
	s.push(addr, s.store.Records(), "digest mismatch")
}

// push sends records to a peer and merges whatever it hands back. The reply
// carries only the peer's side of the diff, so the common case (one missed
// change) moves one record each way.
func (s *Syncer) push(addr string, recs []flag.Record, why string) {
	body, err := json.Marshal(recs)
	if err != nil {
		slog.Error("sync encode failed", "peer", addr, "err", err)
		return
	}
	rep, err := s.call(addr, s.reply(envelope{Type: typeState, Node: s.NodeID, Records: body}), typeStateReply)
	if err != nil {
		slog.Debug("sync push failed", "peer", addr, "err", err)
		return
	}
	back, err := s.apply(rep.Records)
	if err != nil {
		slog.Warn("sync reply rejected", "peer", addr, "err", err)
		return
	}
	slog.Debug("sync push", "peer", addr, "why", why, "sent", len(recs), "applied", rep.Applied, "pulled", back)
}

// Announce pushes one locally written record to every peer immediately instead of
// waiting for the next tick. Best effort: the next round repairs what a dropped
// push missed.
func (s *Syncer) Announce(rec flag.Record) {
	body, err := json.Marshal([]flag.Record{rec})
	if err != nil {
		slog.Error("announce encode failed", "key", rec.Key, "err", err)
		return
	}
	for _, p := range s.Peers {
		go func(p string) {
			rep, err := s.call(p, s.reply(envelope{Type: typeState, Node: s.NodeID, Records: body}), typeStateReply)
			if err != nil {
				slog.Debug("announce failed", "peer", p, "key", rec.Key, "err", err)
				return
			}
			if _, err := s.apply(rep.Records); err != nil {
				slog.Warn("announce reply rejected", "peer", p, "err", err)
			}
		}(p)
	}
}

// apply merges a raw record list from a peer, skipping records that lose.
func (s *Syncer) apply(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	var recs []flag.Record
	if err := json.Unmarshal(raw, &recs); err != nil {
		return 0, err
	}
	n := 0
	for _, r := range recs {
		ok, err := s.store.Apply(r)
		if err != nil {
			slog.Warn("sync record rejected", "key", r.Key, "err", err)
			continue
		}
		if ok {
			n++
		}
	}
	return n, nil
}

// CatchUp pulls from peers until this node reaches minRev or wait elapses. It
// backs the OFREP fence: a caller that has seen a newer revision elsewhere gets
// an answer that is not older, or the best this node has once the wait is up.
func (s *Syncer) CatchUp(minRev int64, wait time.Duration) bool {
	if minRev <= 0 || s.store.Revision() >= minRev {
		return true
	}
	select {
	case s.catch <- struct{}{}:
		defer func() { <-s.catch }()
		s.round()
	default:
		// Another caller is already pulling; just wait for its result.
	}
	deadline := time.Now().Add(wait)
	for {
		if s.store.Revision() >= minRev {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}
