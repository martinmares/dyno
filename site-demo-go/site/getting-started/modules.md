---
title: Modules & Dependencies
weight: 11
---

# Modules & Dependencies

## Semantic versioning

Go modules follow semver strictly. A `v2+` module **must** change its import path:

```go
// v1
import "github.com/you/lib"

// v2+
import "github.com/you/lib/v2"
```

## Replacing a dependency locally

Useful during development of two modules in parallel:

```
// go.mod
replace github.com/you/lib => ../lib
```

Remove the `replace` directive before committing or releasing.

## Private modules

Set `GONOSUMCHECK` and `GOFLAGS` in CI for private registries:

```bash
export GONOSUMCHECK="gitlab.internal/*"
export GOFLAGS="-mod=mod"
export GOPROXY="https://proxy.golang.org,direct"
```

## Checking for vulnerabilities

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

## Upgrading dependencies

```bash
# Upgrade all dependencies to latest minor/patch
go get -u ./...

# Upgrade one package
go get github.com/some/package@latest

# Clean up
go mod tidy
```
