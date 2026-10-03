htmx_url                := "https://unpkg.com/htmx.org@latest/dist/htmx.min.js"
mermaid_url             := "https://cdn.jsdelivr.net/npm/mermaid@latest/dist/mermaid.min.js"
tabler_version          := "1.4.0"
tabler_css_url          := "https://cdn.jsdelivr.net/npm/@tabler/core@{{ tabler_version }}/dist/css/tabler.min.css"
tabler_js_url           := "https://cdn.jsdelivr.net/npm/@tabler/core@{{ tabler_version }}/dist/js/tabler.min.js"
dyno_version            := `tr -d '\n' < VERSION`

default:
    @just --list

editor-assets:
    npm ci
    npm run build:editor

assets-refresh:
    curl -sL "{{ htmx_url }}" -o assets/htmx.min.js
    curl -sL "{{ mermaid_url }}" -o assets/mermaid.min.js
    curl -fsSL "{{ tabler_css_url }}" -o assets/tabler.min.css
    curl -fsSL "{{ tabler_js_url }}" -o assets/tabler.min.js

build-macos:
    #!/usr/bin/env sh
    LD_FLAGS="-X main.version={{ dyno_version }} -X main.buildCommit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    GOOS=darwin GOARCH=arm64 go build -ldflags "$LD_FLAGS" -o dyno-macos .
    echo "-> dyno-macos"

build-linux:
    #!/usr/bin/env sh
    LD_FLAGS="-X main.version={{ dyno_version }} -X main.buildCommit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    GOOS=linux GOARCH=amd64 go build -ldflags "$LD_FLAGS" -o dyno-linux .
    echo "-> dyno-linux"

build-windows:
    #!/usr/bin/env sh
    LD_FLAGS="-X main.version={{ dyno_version }} -X main.buildCommit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    GOOS=windows GOARCH=amd64 go build -ldflags "$LD_FLAGS" -o dyno-windows.exe .
    echo "-> dyno-windows.exe"

release-macos: assets-refresh
    #!/usr/bin/env sh
    LD_FLAGS="-X main.version={{ dyno_version }} -X main.buildCommit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    GOOS=darwin GOARCH=arm64 go build -ldflags "$LD_FLAGS" -o dyno-macos .
    echo "-> dyno-macos"

release-linux: assets-refresh
    #!/usr/bin/env sh
    LD_FLAGS="-X main.version={{ dyno_version }} -X main.buildCommit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    GOOS=linux GOARCH=amd64 go build -ldflags "$LD_FLAGS" -o dyno-linux .
    echo "-> dyno-linux"

release-windows: assets-refresh
    #!/usr/bin/env sh
    LD_FLAGS="-X main.version={{ dyno_version }} -X main.buildCommit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    GOOS=windows GOARCH=amd64 go build -ldflags "$LD_FLAGS" -o dyno-windows.exe .
    echo "-> dyno-windows.exe"

# Build a minimal self-contained Linux image with one documentation site.
# Usage: just site-image registry.example.com/docs/my-site:tag /path/to/site
site-image image site_root:
    #!/usr/bin/env sh
    set -eu

    SITE=$(cd "{{ site_root }}" && pwd)
    PLATFORM=${DOCKER_DEFAULT_PLATFORM:-linux/amd64}
    case "$PLATFORM" in
        linux/amd64) GOARCH=amd64 ;;
        linux/arm64) GOARCH=arm64 ;;
        *) echo "unsupported platform: $PLATFORM (supported: linux/amd64, linux/arm64)" >&2; exit 1 ;;
    esac

    CONTEXT=$(mktemp -d "${TMPDIR:-/tmp}/dyno-site-image.XXXXXX")
    trap 'rm -rf "$CONTEXT"' EXIT INT TERM
    mkdir -p "$CONTEXT/site" "$CONTEXT/root"
    cp LICENSE "$CONTEXT/LICENSE"

    # Include dotfiles, _downloads, and every other file supplied by the site.
    cp -a "$SITE/." "$CONTEXT/site/"
    if [ ! -f "$SITE/dyno.yaml" ] && [ -f "$(dirname "$SITE")/dyno.yaml" ]; then
        cp "$(dirname "$SITE")/dyno.yaml" "$CONTEXT/root/dyno.yaml"
        echo "using parent config: $(dirname "$SITE")/dyno.yaml"
    fi

    VERSION=$(tr -d '\n' < VERSION)
    COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
    BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    LD_FLAGS="-s -w -X main.version=$VERSION -X main.buildCommit=$COMMIT -X main.buildDate=$BUILD_DATE"
    CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -trimpath -ldflags "$LD_FLAGS" -o "$CONTEXT/dyno" .

    docker build \
        --platform "$PLATFORM" \
        --build-arg "DYNO_VERSION=$VERSION" \
        --build-arg "DYNO_COMMIT=$COMMIT" \
        -f Dockerfile.site \
        -t "{{ image }}" \
        "$CONTEXT"
    echo "-> {{ image }} ($PLATFORM, site: $SITE)"
