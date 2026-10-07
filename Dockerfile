# web build
FROM oven/bun:1 AS web
WORKDIR /repo/apps/web
COPY apps/web/package.json apps/web/bun.lock ./
RUN bun install --frozen-lockfile
COPY apps/web/ ./
RUN bun run build

# api build
FROM golang:1.26 AS api
WORKDIR /app
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api/ ./
RUN CGO_ENABLED=1 go build -o /server ./cmd/server

# single runtime: Go serves API + OFREP + web dist from disk.
# Configure via env, never bake secrets: PORT, TCP_PORT, DB_PATH, JWT_SECRET.
FROM debian:12-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=api /server /app/server
COPY apps/api/migrations /app/migrations
COPY packages/contracts /app/contracts
COPY --from=web /repo/apps/web/dist /app/web/dist
# Defaults only. Compose injects real values (see docker-compose.yml).
ENV PORT=8080 TCP_PORT=9090 DB_PATH=/data/waver.db
VOLUME /data
EXPOSE 8080 9090
CMD ["/app/server"]
