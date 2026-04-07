---
title: Getting Started
description: Everything you need to get up and running with Dyno in minutes.
weight: 1
---

# Getting Started

Everything you need to get up and running with Dyno in minutes.

## Installation

Download the binary for your platform from the releases page, or build from source:

```bash
git clone https://github.com/mares/dyno
cd dyno
go build -o dyno .
```

## Directory Structure

Organize your documentation in a `site/` directory:

```
my-docs/
├── site/
│   ├── index.md              ← Home page
│   ├── getting-started/
│   │   ├── index.md          ← Section landing page
│   │   └── configuration.md
│   └── api/
│       └── reference.md
└── dyno                      ← binary
```

## Running

```bash
cd my-docs
./dyno
# or with options:
./dyno --port 8080
```

## Navigation

The left sidebar is **automatically generated** from your directory structure. No configuration needed.

- Directories become **sections** (with an optional landing page via `index.md`)
- Markdown files become **pages**
- Prefix files with numbers to control order: `01-intro.md`, `02-setup.md`

## Images

![](images/2026-04-06-20-54-50.png)
