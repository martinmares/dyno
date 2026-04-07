# API Reference

Dyno exposes a simple HTTP API for advanced integrations.

## Endpoints

### `GET /docs/{path}`

Serves a documentation page.

**Headers:**
- `HX-Request: true` — returns only the page content partial (used by HTMX navigation)

**Response:** HTML page or content partial

---

### `GET /search`

Full-text search across all documentation pages.

**Query Parameters:**
- `q` (string) — search query

**Example:**
```
GET /search?q=installation
```

**Response:** HTML search results partial with highlighted matches.

---

### `GET /healthz`

Health check endpoint for monitoring and load balancers.

**Response:** `200 OK` with body `OK`

---

### `GET /assets/{file}`

Serves static assets (CSS, JavaScript) embedded in the binary.

## Search Highlighting

Search results include `<mark>` tags around matched terms:

```html
<p>See the <mark>installation</mark> guide for details.</p>
```

These are styled with a yellow background by default (customizable via CSS).
