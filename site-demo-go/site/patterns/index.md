---
title: Patterns
weight: 20
---

# Patterns

## Error handling

Always wrap errors with context so the call site is clear without a stack trace:

```go
if err := store.Save(ctx, item); err != nil {
    return fmt.Errorf("save item %d: %w", item.ID, err)
}
```

Use `errors.Is` and `errors.As` to inspect wrapped errors:

```go
var notFound *store.NotFoundError
if errors.As(err, &notFound) {
    // handle specifically
}
```

## Sentinel errors

Define sentinel errors as package-level variables, not strings:

```go
var (
    ErrNotFound   = errors.New("not found")
    ErrPermission = errors.New("permission denied")
)
```

## Functional options

Functional options keep constructors clean and extensible without breaking callers:

```go
type Server struct {
    timeout time.Duration
    logger  *slog.Logger
}

type Option func(*Server)

func WithTimeout(d time.Duration) Option {
    return func(s *Server) { s.timeout = d }
}

func New(opts ...Option) *Server {
    s := &Server{timeout: 30 * time.Second}
    for _, o := range opts {
        o(s)
    }
    return s
}
```

## Context propagation

Always pass `context.Context` as the **first** argument to functions that do I/O:

```go
func (s *Store) FindUser(ctx context.Context, id int64) (*User, error) { ... }
```

Never store a context in a struct — pass it per call.
