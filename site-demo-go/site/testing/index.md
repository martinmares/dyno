---
title: Testing
weight: 40
---

# Testing

## Table-driven tests

The canonical Go testing pattern — covers many cases with minimal boilerplate:

```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name     string
        a, b     int
        want     int
    }{
        {"positive", 1, 2, 3},
        {"negative", -1, -2, -3},
        {"zero", 0, 0, 0},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Add(tt.a, tt.b)
            if got != tt.want {
                t.Errorf("Add(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
            }
        })
    }
}
```

## testify/assert

`github.com/stretchr/testify` reduces assertion boilerplate significantly:

```go
import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestUser(t *testing.T) {
    u, err := NewUser("alice@example.com")
    require.NoError(t, err)          // stops test on failure
    assert.Equal(t, "alice", u.Name) // continues on failure
    assert.NotEmpty(t, u.ID)
}
```

## Benchmarks

```go
func BenchmarkJSON(b *testing.B) {
    data := []byte(`{"id":1,"name":"alice"}`)
    b.ResetTimer()
    for b.N > 0 {
        b.N--
        var v map[string]any
        _ = json.Unmarshal(data, &v)
    }
}
```

Run with:
```bash
go test -bench=. -benchmem ./...
```

## Fuzz testing

```go
func FuzzParseDate(f *testing.F) {
    f.Add("2024-01-15")
    f.Fuzz(func(t *testing.T, s string) {
        // must not panic
        _, _ = ParseDate(s)
    })
}
```

```bash
go test -fuzz=FuzzParseDate -fuzztime=30s ./...
```

## Test helpers

Extract repeated setup into helpers — call `t.Helper()` so failures point to the caller:

```go
func mustCreateUser(t *testing.T, db *sql.DB, email string) *User {
    t.Helper()
    u, err := CreateUser(db, email)
    if err != nil {
        t.Fatalf("mustCreateUser: %v", err)
    }
    return u
}
```

## Coverage

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

> [!TIP]
> Aim for coverage of **critical paths**, not 100%. A test that just exercises a function for the sake of the metric is noise.
