---
title: D2 Diagrams
description: Server-side rendered architecture diagrams using D2
weight: 30
---

# D2 Diagrams

[D2](https://d2lang.com) is a modern diagram scripting language. Unlike Mermaid (which renders in the browser), D2 diagrams are rendered **server-side** to inline SVG — no JavaScript required, instant display, perfect dark/light mode support.

## Basic flow

```d2
User -> "Web Browser": opens docs
"Web Browser" -> "dyno server": HTTP GET /docs/page
"dyno server" -> "dyno server": renders Markdown + D2
"dyno server" -> "Web Browser": HTML with inline SVG
```

## Architecture example

```d2
direction: right

client: Browser {
  shape: rectangle
}

server: dyno {
  shape: rectangle
  md: Markdown renderer
  d2: D2 renderer
  nav: Navigation
  search: Search index
}

files: Site files {
  shape: cylinder
}

client -> server: HTTP request
server.md -> files: reads .md
server.d2 -> files: reads .md
server -> client: HTML + SVG
```

## Containers and nesting

```d2
backend: Backend {
  api: API server
  db: PostgreSQL
  cache: Redis

  api -> db: queries
  api -> cache: caches
}

frontend: Frontend {
  app: React app
  cdn: CDN
}

frontend.app -> backend.api: REST / GraphQL
```

## Syntax reference

| Element | Syntax |
|---------|--------|
| Node | `name` |
| Arrow | `a -> b` |
| Label | `a -> b: label` |
| Container | `group: { ... }` |
| Direction | `direction: right` |
| Shape | `node.shape: rectangle` |

> [!TIP]
> D2 supports many shapes: `rectangle`, `oval`, `cylinder`, `diamond`, `hexagon`, `cloud`, and more.
