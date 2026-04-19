---
title: Getting Started
weight: 10
---

# Getting Started

## Project layout

The Go community settled on a pragmatic layout that scales from a single-file CLI to a large microservice:

```
myservice/
├── cmd/
│   └── myservice/
│       └── main.go       # binary entry point — keep tiny
├── internal/
│   ├── config/
│   ├── handler/
│   └── store/
├── pkg/                  # code safe to import by external packages
├── go.mod
├── go.sum
└── Makefile
```

> [!NOTE]
> There is no official blessed layout. The above is a widely used convention, not a Go specification.

## Module setup

```bash
# Create a new module
go mod init github.com/you/myservice

# Add a dependency
go get github.com/some/package@v1.2.3

# Remove unused dependencies
go mod tidy

# Vendor dependencies (useful in CI/CD without internet)
go mod vendor
```

## Toolchain version pinning

Pin the Go toolchain version in `go.mod` to prevent surprise upgrades:

```
go 1.22.3
toolchain go1.22.3
```

## Environment variables

Use `os.LookupEnv` instead of `os.Getenv` when the distinction between "not set" and "empty string" matters:

```go
val, ok := os.LookupEnv("DATABASE_URL")
if !ok {
    log.Fatal("DATABASE_URL is required")
}
```
