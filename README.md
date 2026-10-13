# Waver

Boolean feature flags with OpenFeature OFREP evaluation. Go backend (DDD),
React admin (clean architecture, Astryx), SQLite, single container. One node is
the default; give a node peers and the same binary becomes a mesh.

## Env

Copy the template and set a real secret. The backend reads the repo-root
`.env` automatically. Real environment variables win over `.env` values.

```
cp .env.example .env
```

| Var                | Default           | Use                                        |
| ------------------ | ----------------- | ------------------------------------------ |
| `PORT`             | `8080`            | API + web port                             |
| `TCP_PORT`         | `9090`            | TCP protocol + peer replication            |
| `DB_PATH`          | `./data/waver.db` | SQLite file                                |
| `JWT_SECRET`       | dev fallback      | Admin sessions (12h)                       |
| `NODE_ID`          | generated         | This node's identity in every clock        |
| `PEERS`            | empty             | `host:port` of peers' TCP ports, comma sep |
| `SYNC_SECRET`      | `JWT_SECRET`      | Shared secret for peer frames              |
| `SYNC_INTERVAL_MS` | `1000`            | Digest round with each peer                |
| `CACHE_MB`         | `64`              | fastcache arena for the flag read path     |

## Run in dev

Terminal 1, API:

```
cd apps/api
go run ./cmd/server
```

Terminal 2, web:

```
cd apps/web
bun install
bun --bun run dev --host 0.0.0.0
```

Open `http://localhost:5173`. Vite proxies `/api` and `/ofrep` to `:8080`.

## First login

First boot prints a generated admin password to stderr. Sign in with it,
then set a new password (min 12 chars). Until then, login returns
`403 MUST_CHANGE`.

To replay first boot, stop the server and delete the DB files:

```
rm ./data/waver.db*
```

## API keys and evaluation

Create a key in the admin UI under API keys. The plain key shows once.
Evaluate server-side only, never from a browser:

```
curl -X POST localhost:8080/ofrep/v1/evaluate/flags/<key> \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"context":{"targetingKey":"user-123"}}'
```

Bulk evaluation: `POST /ofrep/v1/evaluate/flags` with the same body.
Revoking a key rejects evaluation immediately.

## Contracts

`packages/contracts/openapi.yaml` is the contract clients read, not decoration.
`go test ./...` in `apps/api` covers the flag register, the cache, and the
replication path. Update both sides together.

Live reference for clients: Swagger UI at `/docs`, raw file at
`/docs/openapi.yaml`. Both public, no auth. Try OFREP calls there.

## Run in Docker

Set `JWT_SECRET` in root `.env` first, then:

```
docker compose up --build
```

One container builds the web UI with Bun, compiles the API with Go, and
serves both from one process. SQLite lives on the `waver-data` volume at
`/data/waver.db`. Ports and secrets inject via `docker-compose.yml`:
`env_file: .env` plus explicit `environment` wins for `DB_PATH`.
First boot prints the generated admin password to container stderr:

```
docker compose logs waver | grep -A4 "FIRST BOOT"
```

For a mesh, run one container per node with its own `waver-data` volume, its own
`PORT`/`TCP_PORT` mapping, and `NODE_ID`/`PEERS` in the environment. The compose
file forwards everything in `.env`, so `PEERS` uses the names peers resolve to
(container names on a compose network, or `host:port` across hosts).

## Cluster mode

Every node keeps the whole flag map in its own sqlite file. No master, no extra
service to run, no shared database. Nodes only tell each other what changed, over
the TCP port they already serve.

Three nodes on one machine:

```
PEERS=127.0.0.1:9091,127.0.0.1:9092 NODE_ID=n1 PORT=8081 TCP_PORT=9091 DB_PATH=./data/n1.db go run ./cmd/server
PEERS=127.0.0.1:9090,127.0.0.1:9092 NODE_ID=n2 PORT=8082 TCP_PORT=9091 DB_PATH=./data/n2.db go run ./cmd/server
PEERS=127.0.0.1:9090,127.0.0.1:9091 NODE_ID=n3 PORT=8083 TCP_PORT=9092 DB_PATH=./data/n3.db go run ./cmd/server
```

Each node's `PEERS` lists the *other* nodes' `TCP_PORT`s, and every node needs
the same `SYNC_SECRET`. `GET /healthz` reports `node`, `revision`, `flags`, and
`peers`; converged nodes report the same `revision`.

How a change travels:

- A write stamps a Lamport clock for that key — one higher than every clock this
  node has seen for it — and goes through the same apply path a peer's record
  takes. Local and remote writes cannot disagree, because they are the same code.
- The writer pushes the record to every peer immediately, so peers have it in
  milliseconds rather than on the next tick.
- Every `SYNC_INTERVAL_MS` a node compares a state digest with each peer. Equal
  digests mean identical keys and clocks, so nothing moves. A mismatch makes the
  node that noticed push its whole state; the peer applies what wins and answers
  with only the records that beat it.
- Newest clock wins and equal clocks break on node id, so every node picks the
  same winner without asking anyone.
- Deletes travel as tombstones, so a node that was offline during a delete cannot
  bring the flag back.
- Reads never wait on the database: `CACHE_MB` of fastcache is filled from
  sqlite at startup and kept current by every write, local or replicated.

Adding a node: start it with `PEERS` pointing at any live node and an empty
database. Its first digest round pulls the full state.

Read-your-writes across nodes: responses report a `revision`
(`metadata.revision` on OFREP responses, `version` per flag). Send a revision you
have already seen as `X-Waver-Min-Rev` (or `min_revision` on a TCP eval line) and
the node pulls from its peers before answering, giving up after a second and
answering with what it has.

Saving a flag with `version` in the body turns an edit that raced another node
into `409` instead of a silent overwrite. The admin UI sends it.

What this does not do:

- Concurrent edits to the same flag are last-writer-wins, by design. Send
  `version` to make that visible as a conflict instead of a lost edit.
- A node cut off from its peers keeps answering with its own values, however
  stale. That is the availability trade, not a failure.
- Every node talks to every address in `PEERS`. That is fine into the dozens of
  nodes; past that, list a few random peers per node instead.
- A state push is a single protocol line, capped at 8MB, which is the ceiling on
  the whole flag set in one frame.
- Tombstones are kept forever, so the database grows by one small row per deleted
  key. Garbage collecting them safely needs a watermark agreed across nodes.
- Nodes need distinct `NODE_ID`s. One is generated and stored on first boot when
  `NODE_ID` is unset, so cloning a data directory without setting `NODE_ID` is
  the one way to get two nodes sharing an identity.
- Only flags replicate. The admin password and API keys live in each node's own
  database, so create an API key on every node that serves evaluation.
