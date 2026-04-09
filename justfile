tailwind_url_linux      := "https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64"
tailwind_url_macos_arm  := "https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64"
tailwind_url_macos_x64  := "https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-x64"
htmx_url                := "https://unpkg.com/htmx.org@latest/dist/htmx.min.js"
mermaid_url             := "https://cdn.jsdelivr.net/npm/mermaid@latest/dist/mermaid.min.js"

default:
    @just --list

_tailwind-install:
    #!/usr/bin/env sh
    if [ -f ./tailwindcss ]; then exit 0; fi
    OS=$(uname -s); ARCH=$(uname -m)
    if [ "$OS" = "Linux" ]; then
        URL="{{ tailwind_url_linux }}"
    elif [ "$ARCH" = "arm64" ]; then
        URL="{{ tailwind_url_macos_arm }}"
    else
        URL="{{ tailwind_url_macos_x64 }}"
    fi
    echo "downloading tailwindcss..."
    curl -sL "$URL" -o tailwindcss && chmod +x tailwindcss

css: _tailwind-install
    ./tailwindcss -i assets/input.css -o assets/tailwind.css --minify

css-watch: _tailwind-install
    ./tailwindcss -i assets/input.css -o assets/tailwind.css --watch

assets-refresh:
    curl -sL "{{ htmx_url }}" -o assets/htmx.min.js
    curl -sL "{{ mermaid_url }}" -o assets/mermaid.min.js

release-macos: assets-refresh css
    GOOS=darwin GOARCH=arm64 go build -o dyno-macos .
    @echo "-> dyno-macos"

release-linux: assets-refresh css
    GOOS=linux GOARCH=amd64 go build -o dyno-linux .
    @echo "-> dyno-linux"

release-windows: assets-refresh css
    GOOS=windows GOARCH=amd64 go build -o dyno-windows.exe .
    @echo "-> dyno-windows.exe"
