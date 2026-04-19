---
title: Docker
weight: 10
---

# Docker

## Multi-stage build for Go

A production-grade `Dockerfile` that produces a minimal final image:

```dockerfile
# ── Build stage ──────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /bin/myservice \
    ./cmd/myservice

# ── Final stage ──────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12

COPY --from=builder /bin/myservice /myservice
USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/myservice"]
```

> [!TIP]
> `distroless/static` has no shell, no package manager, no libc. Attack surface is minimal.

## .dockerignore

Always include a `.dockerignore` to keep the build context small:

```
.git
.github
**/*.md
bin/
vendor/
*_test.go
```

## Image hygiene checklist

- Run as a non-root user
- Use a specific image tag, never `latest`
- Set `HEALTHCHECK` or rely on orchestrator probes
- Label images with build metadata

```dockerfile
LABEL org.opencontainers.image.source="https://github.com/you/myservice"
LABEL org.opencontainers.image.revision="${GIT_COMMIT}"
```

## Useful commands

```bash
# Build with a tag
docker build -t myservice:$(git rev-parse --short HEAD) .

# Check image layers and sizes
docker history myservice:latest

# Scan for vulnerabilities
docker scout cves myservice:latest

# Run interactively for debugging
docker run --rm -it --entrypoint sh myservice:latest
```
