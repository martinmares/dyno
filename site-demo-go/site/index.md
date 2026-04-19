---
title: Go Cookbook
weight: 0
---

# Go Cookbook

Practical patterns, idioms and tooling for building production Go services. This cookbook focuses on real-world code you can copy, adapt and ship — not toy examples.

> [!TIP]
> All code snippets are written for Go 1.22+. Most patterns work from Go 1.18 onwards.

## What's inside

| Section | What you'll find |
|---|---|
| [Getting Started](getting-started/) | Project layout, modules, toolchain setup |
| [Patterns](patterns/) | Error handling, concurrency, interfaces |
| [Tooling](tooling/) | `go tool`, linters, build flags, Makefile |
| [Testing](testing/) | Table tests, fuzz, benchmarks, mocks |

## Philosophy

- **Standard library first** — reach for a third-party package only when the stdlib genuinely can't do the job.
- **Errors are values** — wrap with context, never discard.
- **Concurrency is not parallelism** — goroutines are cheap; synchronization is not.
- **Make the zero value useful** — design types so the default state is valid.
