-- Replicated flag state. The migration runner re-executes every file on boot,
-- so every statement here stays idempotent.
--
-- node_clock is this node's own Lamport counter plus its identity. The row is
-- seeded at 0/'' and the server claims node_id on first boot when NODE_ID is
-- unset.
CREATE TABLE IF NOT EXISTS node_clock (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  lamport INTEGER NOT NULL DEFAULT 0,
  node_id TEXT NOT NULL DEFAULT ''
);
INSERT OR IGNORE INTO node_clock (id, lamport, node_id) VALUES (1, 0, '');

-- key_clock holds one row per key this node has ever seen, live or deleted: the
-- winning clock plus whether the winner is a tombstone. The value itself stays
-- in flags, so a peer only needs key + clock + deleted to decide who wins.
CREATE TABLE IF NOT EXISTS key_clock (
  key     TEXT PRIMARY KEY,
  lamport INTEGER NOT NULL,
  node_id TEXT NOT NULL,
  deleted INTEGER NOT NULL DEFAULT 0,
  at      INTEGER NOT NULL
);
