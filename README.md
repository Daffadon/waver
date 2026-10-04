# Waver

Boolean feature flags with OpenFeature OFREP evaluation. Go backend (DDD),
React admin (clean architecture, Astryx), SQLite, single container.

## Env

Copy the template and set a real secret. The backend reads the repo-root
`.env` automatically. Real environment variables win over `.env` values.

```
cp .env.example .env
```

| Var          | Default            | Use                  |
| ------------ | ------------------ | -------------------- |
| `PORT`       | `8080`             | API + web port       |
| `DB_PATH`    | `./data/waver.db`  | SQLite file          |
| `JWT_SECRET` | dev fallback       | Admin sessions (12h) |

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

`packages/contracts/openapi.yaml` is enforced, not decoration.
`go test ./...` in `apps/api` probes every documented route and fails on
undocumented or unimplemented ones. Update both sides together.

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
