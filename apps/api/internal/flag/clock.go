package flag

import "encoding/binary"

// Clock is a Lamport timestamp. A write bumps it past every clock this node has
// seen for that key, and the writing node breaks ties, so any two nodes that see
// the same two records pick the same winner without talking to a third party.
type Clock struct {
	Lamport int64  `json:"lamport"`
	Node    string `json:"node"`
}

// After reports whether c supersedes o. An equal clock is not after, which is
// what makes applying the same record twice a no-op.
func (c Clock) After(o Clock) bool {
	if c.Lamport != o.Lamport {
		return c.Lamport > o.Lamport
	}
	return c.Node > o.Node
}

// Record is one key's replicable state: the winning clock plus either the flag
// or a tombstone. Tombstones replicate like any value (and are kept forever) so
// a node that was offline during a delete cannot resurrect the flag.
type Record struct {
	Key     string `json:"key"`
	Clock   Clock  `json:"clock"`
	Deleted bool   `json:"deleted,omitempty"`
	Flag    *Flag  `json:"flag,omitempty"`
	At      int64  `json:"at"`
}

// clockDigest is the hand-rolled FNV-1a 64 of one key's metadata. Digests are
// summed, so two nodes holding the same keys with the same clocks and tombstones
// land on the same number whatever order the records arrived in. The flag payload
// is deliberately excluded: the clock is what decides, and it changes on every
// write.
func clockDigest(key string, c Clock, deleted bool) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	h := uint64(offset)
	for i := range len(key) {
		h = (h ^ uint64(key[i])) * prime
	}
	var buf [9]byte
	binary.LittleEndian.PutUint64(buf[:8], uint64(c.Lamport))
	if deleted {
		buf[8] = 1
	}
	for _, b := range buf {
		h = (h ^ uint64(b)) * prime
	}
	for i := range len(c.Node) {
		h = (h ^ uint64(c.Node[i])) * prime
	}
	return h
}
