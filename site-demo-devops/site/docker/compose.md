---
title: Docker Compose
weight: 11
---

# Docker Compose

## Local development stack

A `compose.yaml` that mirrors production dependencies locally:

```yaml
services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://dev:dev@db:5432/myapp?sslmode=disable
      - REDIS_URL=redis://cache:6379
    depends_on:
      db:
        condition: service_healthy
      cache:
        condition: service_started
    develop:
      watch:
        - action: rebuild
          path: .

  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: dev
      POSTGRES_PASSWORD: dev
      POSTGRES_DB: myapp
    volumes:
      - pg_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U dev"]
      interval: 5s
      timeout: 3s
      retries: 5

  cache:
    image: redis:7-alpine
    command: redis-server --maxmemory 128mb --maxmemory-policy allkeys-lru

volumes:
  pg_data:
```

## Useful commands

```bash
# Start all services
docker compose up -d

# Live reload with watch mode (Compose 2.22+)
docker compose watch

# View logs from all services
docker compose logs -f

# Run a one-off command against the db
docker compose exec db psql -U dev myapp

# Tear down everything including volumes
docker compose down -v
```

## Overrides for CI

Use `compose.override.yaml` to add CI-specific settings without touching the base file:

```yaml
# compose.override.yaml  (committed — safe for CI)
services:
  app:
    environment:
      - LOG_FORMAT=json
  db:
    tmpfs:
      - /var/lib/postgresql/data  # faster, ephemeral in CI
```
