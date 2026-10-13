// Package sync replicates the flag register between peer nodes. Frames are JSON
// lines over a node's TCP port, the same listener the API protocol uses: one
// frame in, one frame out.
//
// Steady state is one tiny digest frame per peer per tick. Only a mismatch moves
// payloads, and the node that noticed the mismatch pushes its whole state, so a
// peer that is far behind or freshly started converges in a single round.
//
// Frames are authenticated with a shared secret (HMAC-SHA256 over the type, node,
// verdict and the record bytes as sent). A frame that fails the MAC is dropped:
// without it, anyone reaching the TCP port could inject or delete flags.
package sync

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/daffadon/waver/internal/flag"
)

// Frame types. Every frame is one JSON line.
const (
	typeDigest      = "sync_digest"
	typeDigestReply = "sync_digest_reply"
	typeState       = "sync_state"
	typeStateReply  = "sync_state_reply"
	typeError       = "sync_error"
)

// framePrefix marks a line as replication rather than API traffic.
const framePrefix = "sync_"

// IsFrame reports whether a line on the TCP port is a replication frame. The TCP
// server uses it to hand the line to the syncer instead of the router.
func IsFrame(t string) bool { return strings.HasPrefix(t, framePrefix) }

// envelope is the wire shape of every frame. Records stay raw so both ends MAC
// the exact bytes that crossed the wire, never a re-marshal.
type envelope struct {
	Type    string          `json:"type"`
	Node    string          `json:"node"`
	Digest  uint64          `json:"digest,omitempty"`
	Equal   bool            `json:"equal,omitempty"`
	Applied int             `json:"applied,omitempty"`
	Records json.RawMessage `json:"records,omitempty"`
	MAC     string          `json:"mac,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// sign covers every field that decides what a frame does, records included, so a
// frame cannot be replayed with a different payload or a different verdict.
func (e envelope) sign(secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s\n%s\n%d\n%d\n%d\n", e.Type, e.Node, e.Digest, btoi(e.Equal), e.Applied)
	mac.Write(e.Records)
	return hex.EncodeToString(mac.Sum(nil))
}

func (e envelope) verify(secret []byte) bool {
	want, err := hex.DecodeString(e.MAC)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s\n%s\n%d\n%d\n%d\n", e.Type, e.Node, e.Digest, btoi(e.Equal), e.Applied)
	mac.Write(e.Records)
	return hmac.Equal(want, mac.Sum(nil))
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// reply signs an outbound frame.
func (s *Syncer) reply(e envelope) envelope {
	e.MAC = e.sign(s.secret)
	return e
}

func errFrame(msg string) envelope { return envelope{Type: typeError, Error: msg} }

// call sends one frame to a peer and returns its verified reply. One connection
// carries exactly one round trip, so a stalled peer cannot hold a socket open.
func (s *Syncer) call(addr string, req envelope, want string) (envelope, error) {
	c, err := net.DialTimeout("tcp", addr, s.timeout)
	if err != nil {
		return envelope{}, err
	}
	defer c.Close()
	if err := c.SetDeadline(time.Now().Add(s.timeout)); err != nil {
		return envelope{}, err
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return envelope{}, err
	}
	if _, err := c.Write(append(raw, '\n')); err != nil {
		return envelope{}, err
	}
	line, err := bufio.NewReaderSize(c, 64<<10).ReadBytes('\n')
	if err != nil {
		return envelope{}, err
	}
	var rep envelope
	if err := json.Unmarshal(line, &rep); err != nil {
		return envelope{}, fmt.Errorf("bad reply: %w", err)
	}
	if rep.Error != "" {
		return rep, errors.New(rep.Error)
	}
	if !rep.verify(s.secret) {
		return envelope{}, errors.New("bad mac")
	}
	if rep.Type != want {
		return envelope{}, fmt.Errorf("unexpected reply %q", rep.Type)
	}
	return rep, nil
}

// Frame handles one inbound replication frame, returning the reply to write back
// and whether the line was a replication frame at all.
func (s *Syncer) Frame(raw []byte) (any, bool) {
	var in envelope
	if err := json.Unmarshal(raw, &in); err != nil {
		return errFrame("bad json"), true
	}
	if !IsFrame(in.Type) {
		return nil, false
	}
	if !in.verify(s.secret) {
		slog.Warn("sync frame rejected", "peer", in.Node, "type", in.Type, "reason", "bad mac")
		return errFrame("bad mac"), true
	}
	switch in.Type {
	case typeDigest:
		digest, _ := s.store.Digest()
		return s.reply(envelope{Type: typeDigestReply, Node: s.NodeID, Digest: digest, Equal: in.Digest == digest}), true
	case typeState:
		var recs []flag.Record
		if err := json.Unmarshal(in.Records, &recs); err != nil {
			return errFrame("bad records"), true
		}
		clocks := make(map[string]flag.Clock, len(recs))
		applied := 0
		for _, r := range recs {
			clocks[r.Key] = r.Clock
			ok, err := s.store.Apply(r)
			if err != nil {
				slog.Warn("sync record rejected", "peer", in.Node, "key", r.Key, "err", err)
				continue
			}
			if ok {
				applied++
			}
		}
		mine := s.store.RecordsNewerThan(clocks)
		body, err := json.Marshal(mine)
		if err != nil {
			return errFrame("encode error"), true
		}
		if applied > 0 {
			slog.Info("sync applied", "peer", in.Node, "records", applied)
		}
		digest, _ := s.store.Digest()
		return s.reply(envelope{Type: typeStateReply, Node: s.NodeID, Digest: digest, Applied: applied, Records: body}), true
	}
	return errFrame("unknown sync frame"), true
}
