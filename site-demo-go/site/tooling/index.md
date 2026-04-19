---
title: Tooling
weight: 30
---

# Tooling

## Essential tools

```bash
# Linter (fast, comprehensive)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Static analysis
go vet ./...

# Format (always use gofmt, never argue about style)
gofmt -w .

# Imports organizer
go install golang.org/x/tools/cmd/goimports@latest
goimports -w .
```

## Recommended `.golangci.yml`

```yaml
linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gofmt
    - goimports
    - misspell
    - revive

linters-settings:
  revive:
    rules:
      - name: exported
```

## Makefile

A minimal Makefile that covers the common workflow:

```makefile
.PHONY: build test lint clean

build:
	go build -ldflags="-s -w" -o bin/myservice ./cmd/myservice

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

run: build
	./bin/myservice
```

## Build flags

```bash
# Strip debug info and symbol table (smaller binary)
go build -ldflags="-s -w" .

# Inject version at build time
go build -ldflags="-X main.version=$(git describe --tags)" .

# Cross-compile for Linux on Mac
GOOS=linux GOARCH=amd64 go build .
```

## go generate

Use `go generate` to run code generators as part of the build:

```go
//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc generate
//go:generate mockgen -source=store.go -destination=mock/store.go
```

Run with:
```bash
go generate ./...
```
