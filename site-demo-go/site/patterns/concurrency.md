---
title: Concurrency
weight: 21
---

# Concurrency

## Worker pool

A simple, bounded worker pool using a buffered channel as a semaphore:

```go
const workers = 8
sem := make(chan struct{}, workers)

for _, item := range items {
    item := item // capture loop variable (Go < 1.22)
    sem <- struct{}{}
    go func() {
        defer func() { <-sem }()
        process(item)
    }()
}

// Wait for all workers to finish
for range workers {
    sem <- struct{}{}
}
```

## errgroup

`golang.org/x/sync/errgroup` is the idiomatic way to run goroutines and collect errors:

```go
g, ctx := errgroup.WithContext(ctx)

g.Go(func() error {
    return fetchUsers(ctx)
})
g.Go(func() error {
    return fetchOrders(ctx)
})

if err := g.Wait(); err != nil {
    return err
}
```

## sync.Once for lazy initialization

```go
type Cache struct {
    once sync.Once
    data map[string]string
}

func (c *Cache) Load() map[string]string {
    c.once.Do(func() {
        c.data = expensiveLoad()
    })
    return c.data
}
```

## Avoiding data races

Run tests with the race detector — it has zero false positives:

```bash
go test -race ./...
```

> [!WARNING]
> The race detector adds significant overhead (~5–10x slower). Use it in CI, not in the production binary.

## Channel direction in signatures

Communicate intent by restricting channel direction in function signatures:

```go
func producer(out chan<- int) { ... } // send-only
func consumer(in <-chan int)  { ... } // receive-only
```
