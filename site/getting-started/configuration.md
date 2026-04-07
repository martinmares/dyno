---
title: Configuration
description: How to configure Dyno using dyno.yaml and CLI flags.
weight: 2
---

# Configuration

Dyno is designed to work without any configuration file. Everything is driven by CLI flags and directory structure.

## CLI Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--port` | `-p` | `3000` | Port to listen on |
| `--dir` | `-d` | `.` | Root directory (must contain `site/`) |
| `--version` | | | Print version and exit |

## Examples

```bash
# Default: serve current directory on port 3000
./dyno

# Custom port
./dyno --port 8080

# Serve a specific directory
./dyno --dir /home/user/my-docs

# Short flags
./dyno -p 8080 -d /home/user/my-docs
```

## Ordering Pages

Prefix filenames with numbers to control sidebar order:

```
site/
├── 01-introduction.md     → "Introduction"
├── 02-installation.md     → "Installation"
└── 03-configuration.md    → "Configuration"
```

The numbers are stripped from the display title automatically.
