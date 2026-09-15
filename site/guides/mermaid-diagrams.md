---
title: Mermaid Diagrams
description: How to embed Mermaid diagrams in your documentation pages.
weight: 1
---

# Mermaid Diagrams

Dyno supports [Mermaid.js](https://mermaid.js.org/) diagrams out of the box. Just use a fenced code block with the `mermaid` language tag.

## Flowchart

```mermaid
flowchart LR
    A[Write Markdown] --> B[Run dyno]
    B --> C{Has mermaid?}
    C -->|Yes| D[Render diagram]
    C -->|No| E[Render HTML]
    D --> F[Beautiful docs]
    E --> F
```

## Sequence Diagram

```mermaid
sequenceDiagram
    participant Browser
    participant Dyno
    participant Filesystem

    Browser->>Dyno: GET /docs/guide
    Dyno->>Filesystem: Read guide.md
    Filesystem-->>Dyno: Markdown source
    Dyno->>Dyno: Render to HTML
    Dyno-->>Browser: HTML response
```

## Entity Relationship

```mermaid
erDiagram
    NavNode {
        string Title
        string FullPath
        string FSPath
        bool IsDir
    }
    NavNode ||--o{ NavNode : children
    SearchIndex ||--o{ Document : indexes
    Document {
        string Path
        string Title
        string Body
    }
```

## Pie Chart

```mermaid
pie title Technology Stack
    "Go (server)" : 60
    "HTMX" : 15
    "Tabler CSS" : 15
    "Mermaid.js" : 10
```
