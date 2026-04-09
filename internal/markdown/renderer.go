package markdown

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	d2log "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/go2"
)

// TOCEntry represents a heading for the table of contents.
type TOCEntry struct {
	Level int
	ID    string
	Text  string
}

// Frontmatter holds optional YAML metadata from the top of a Markdown file.
type Frontmatter struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Draft       bool   `yaml:"draft"`
	Weight      int    `yaml:"weight"` // for future custom ordering
}

// Result holds the rendered HTML and extracted metadata.
type Result struct {
	HTML        string
	TOC         []TOCEntry
	Title       string
	Frontmatter Frontmatter
}

// Renderer wraps goldmark with our configuration.
type Renderer struct {
	md       goldmark.Markdown
	lightCSS string
	darkCSS  string
}

// NewRenderer creates a configured Renderer and pre-generates chroma CSS.
func NewRenderer() (*Renderer, error) {
	lightBuf := &bytes.Buffer{}

	lightFmt := chromahtml.New(chromahtml.WithClasses(true))
	if err := lightFmt.WriteCSS(lightBuf, styles.Get("github-dark")); err != nil {
		return nil, err
	}

	// Dark mode uses the same style — dark code blocks work in both modes
	rawDark := &bytes.Buffer{}
	darkFmt := chromahtml.New(chromahtml.WithClasses(true))
	if err := darkFmt.WriteCSS(rawDark, styles.Get("github-dark")); err != nil {
		return nil, err
	}
	// Re-scope every chroma rule under "html.dark" by prefixing each selector.
	// Chroma CSS is well-structured: each rule is ".chroma ..." or ".chroma .xx { ... }".
	// We replace ".chroma" with "html.dark .chroma" globally.
	darkBuf := bytes.NewBufferString(
		strings.ReplaceAll(rawDark.String(), ".chroma", "html.dark .chroma"),
	)

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			extension.DefinitionList,
			highlighting.NewHighlighting(
				highlighting.WithStyle("github-dark"),
				highlighting.WithGuessLanguage(true),
				highlighting.WithFormatOptions(
					chromahtml.WithClasses(true),
				),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithAttribute(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithUnsafe(),
		),
	)

	return &Renderer{
		md:       md,
		lightCSS: lightBuf.String(),
		darkCSS:  darkBuf.String(),
	}, nil
}

// LightCSS returns the chroma light mode CSS.
func (r *Renderer) LightCSS() string { return r.lightCSS }

// DarkCSS returns the chroma dark mode CSS.
func (r *Renderer) DarkCSS() string { return r.darkCSS }

// Render converts Markdown source to HTML, transforms mermaid blocks,
// and extracts TOC entries and the page title.
func (r *Renderer) Render(src []byte) (*Result, error) {
	fm, body := parseFrontmatter(src)

	// API: replace ```api fences with placeholder tokens BEFORE goldmark.
	apiBlocks := map[string]string{}
	preprocessed := replaceAPIWithPlaceholders(body, apiBlocks)

	// D2: replace ```d2 fences with placeholder tokens BEFORE goldmark so the
	// SVG content (which contains <style>, CDATA, etc.) is never parsed as
	// Markdown. The rendered SVGs are stored in a map keyed by placeholder.
	d2SVGs := map[string]string{}
	preprocessed = replaceD2WithPlaceholders(preprocessed, d2SVGs)

	// Mermaid: replace ```mermaid with <pre class="mermaid"> before goldmark.
	preprocessed = extractMermaidBlocks(preprocessed)

	var buf bytes.Buffer
	if err := r.md.Convert(preprocessed, &buf); err != nil {
		return nil, err
	}

	// Restore placeholders: D2 SVGs, then API widgets.
	htmlStr := restoreD2Placeholders(buf.String(), d2SVGs)
	htmlStr = restorePlaceholders(htmlStr, apiBlocks)
	htmlStr = transformCallouts(addAnchorLinks(htmlStr))
	toc := extractTOC(htmlStr)
	title := extractTitle(htmlStr)
	if title == "" && fm.Title != "" {
		title = fm.Title
	}

	return &Result{
		HTML:        htmlStr,
		TOC:         toc,
		Title:       title,
		Frontmatter: fm,
	}, nil
}

var frontmatterRe = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?`)

// parseFrontmatter strips YAML front matter from src and returns it parsed + the remaining body.
func parseFrontmatter(src []byte) (Frontmatter, []byte) {
	var fm Frontmatter
	m := frontmatterRe.FindSubmatch(src)
	if m == nil {
		return fm, src
	}
	_ = yaml.Unmarshal(m[1], &fm)
	return fm, src[len(m[0]):]
}

// RenderString is a convenience wrapper.
func (r *Renderer) RenderString(src string) (*Result, error) {
	return r.Render([]byte(src))
}

// ToPlainText renders Markdown and strips all HTML tags, for search indexing.
func (r *Renderer) ToPlainText(src []byte) (string, error) {
	res, err := r.Render(src)
	if err != nil {
		return "", err
	}
	return stripTags(res.HTML), nil
}

// d2FenceRe matches ```d2 ... ``` fenced code blocks in Markdown source.
var d2FenceRe = regexp.MustCompile("(?ms)^```d2\\s*\n(.*?)\n```")
var nestedD2SVGRe = regexp.MustCompile(`(?s)\A<svg\b([^>]*)>\s*<svg\b([^>]*)>(.*)</svg>\s*</svg>\z`)
var d2BackgroundRectRe = regexp.MustCompile(`(?s)\A(<svg\b[^>]*>)\s*<rect\b[^>]*stroke-width="0"[^>]*/>(.*)\z`)

// renderD2 compiles D2 source to two inline SVGs (light + dark) wrapped in a
// container div. CSS shows/hides each via the html.dark class, matching the
// site's theme toggle. Two separate renders avoids any media-query or CSS
// nesting issues with inline SVG.
func renderD2(src string) (string, error) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return "", fmt.Errorf("d2 ruler: %w", err)
	}

	pad := int64(20)
	// Inject a silent logger so D2 doesn't print WARN stacktraces to stderr.
	ctx := d2log.With(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
		return func(c context.Context, g *d2graph.Graph) error {
			return d2dagrelayout.Layout(c, g, nil)
		}, nil
	}
	compileOpts := &d2lib.CompileOptions{
		Ruler:          ruler,
		Layout:         go2.Pointer("dagre"),
		LayoutResolver: layoutResolver,
	}

	renderSingle := func(themeID int64) (string, error) {
		opts := &d2svg.RenderOpts{ThemeID: &themeID, Pad: &pad}
		diagram, _, err := d2lib.Compile(ctx, src, compileOpts, opts)
		if err != nil {
			return "", err
		}
		b, err := d2svg.Render(diagram, opts)
		return string(b), err
	}

	lightSVG, err := renderSingle(0) // Default light
	if err != nil {
		return "", fmt.Errorf("d2 light render: %w", err)
	}
	darkSVG, err := renderSingle(200) // Dark Mauve
	if err != nil {
		return "", fmt.Errorf("d2 dark render: %w", err)
	}

	return "<div class=\"d2-diagram\">" +
		"<span class=\"d2-light\">" + normalizeD2SVG(lightSVG) + "</span>" +
		"<span class=\"d2-dark\">" + normalizeD2SVG(darkSVG) + "</span>" +
		"</div>", nil
}

func normalizeD2SVG(s string) string {
	if after, ok := strings.CutPrefix(s, `<?xml version="1.0" encoding="utf-8"?>`); ok {
		s = strings.TrimSpace(after)
	}

	// D2 emits an outer wrapper <svg> around the real diagram SVG. Flattening
	// it avoids browser quirks with nested inline SVG roots.
	if subs := nestedD2SVGRe.FindStringSubmatch(s); len(subs) == 4 {
		outerAttrs := regexp.MustCompile(`\s+viewBox="[^"]*"`).ReplaceAllString(subs[1], "")
		s = "<svg" + outerAttrs + subs[2] + ">" + subs[3] + "</svg>"
	}

	// Let the page background show through instead of keeping D2's full-canvas
	// background rect, which looks like a mismatched panel in dark mode.
	if subs := d2BackgroundRectRe.FindStringSubmatch(s); len(subs) == 3 {
		s = subs[1] + subs[2]
	}

	return strings.Replace(s, "<svg ", `<svg style="max-width:100%;height:auto;" `, 1)
}

// replaceD2WithPlaceholders replaces ```d2 fences with opaque placeholder
// tokens and stores the rendered SVG HTML in svgs. This keeps SVG content
// (which has <style>, CDATA, etc.) away from goldmark entirely.
func replaceD2WithPlaceholders(src []byte, svgs map[string]string) []byte {
	counter := 0
	return d2FenceRe.ReplaceAllFunc(src, func(match []byte) []byte {
		subs := d2FenceRe.FindSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		key := fmt.Sprintf("D2PLACEHOLDER%d", counter)
		counter++
		svg, err := renderD2(string(subs[1]))
		if err != nil {
			svg = `<div class="callout callout-danger"><div class="callout-title">🚨 D2 render error</div><div class="callout-body"><p>` + err.Error() + `</p></div></div>`
		}
		svgs[key] = svg
		// Emit as a raw HTML block so goldmark passes it through unchanged.
		// The token contains no HTML-special chars so goldmark won't mangle it.
		return []byte("<div>" + key + "</div>")
	})
}

// restoreD2Placeholders swaps placeholder tokens back to their SVG HTML.
func restoreD2Placeholders(html string, svgs map[string]string) string {
	for key, svg := range svgs {
		html = strings.ReplaceAll(html, "<div>"+key+"</div>", svg)
	}
	return html
}

// mermaidFenceRe matches ```mermaid ... ``` fenced code blocks in Markdown source.
var mermaidFenceRe = regexp.MustCompile("(?ms)^```mermaid\\s*\n(.*?)\n```")

// emptyLineRe matches blank lines (goldmark ends an HTML block on blank line).
var emptyLineRe = regexp.MustCompile(`(?m)^\s*$`)

// extractMermaidBlocks replaces ```mermaid fences in Markdown source with raw
// HTML before goldmark parses the document. This prevents chroma from
// syntax-highlighting mermaid content.
// We use <pre class="mermaid"> because goldmark's HTML block type 1 (<pre>)
// is only terminated by </pre>, so blank lines inside the diagram are safe.
// app.js converts <pre class="mermaid"> back to <div class="mermaid"> before
// calling mermaid.run(), or we just point mermaid at pre.mermaid directly.
func extractMermaidBlocks(src []byte) []byte {
	return mermaidFenceRe.ReplaceAllFunc(src, func(match []byte) []byte {
		subs := mermaidFenceRe.FindSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		return append([]byte("<pre class=\"mermaid\">"), append(subs[1], []byte("</pre>")...)...)
	})
}

var (
	headingRe    = regexp.MustCompile(`(?s)<h([23])[^>]*\sid="([^"]+)"[^>]*>(.*?)</h[23]>`)
	allHeadingRe = regexp.MustCompile(`(?s)<(h[1-6])[^>]*\sid="([^"]+)"[^>]*>(.*?)</h[1-6]>`)
	anchorLinkRe = regexp.MustCompile(`(?s)<a[^>]*class="anchor-link"[^>]*>.*?</a>`)
	tagRe        = regexp.MustCompile(`<[^>]+>`)
)

// addAnchorLinks injects a # anchor link to the left of every heading that has an id.
// The anchor is placed before the text content so it appears on the left side.
func addAnchorLinks(htmlStr string) string {
	return allHeadingRe.ReplaceAllStringFunc(htmlStr, func(match string) string {
		subs := allHeadingRe.FindStringSubmatch(match)
		if len(subs) < 4 {
			return match
		}
		tag, id, inner := subs[1], subs[2], subs[3]
		anchor := `<a href="#` + id + `" class="anchor-link" aria-hidden="true"><svg width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M10 13a5 5 0 007.54.54l3-3a5 5 0 00-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 00-7.54-.54l-3 3a5 5 0 007.07 7.07l1.71-1.71"/></svg></a>`
		return `<` + tag + ` id="` + id + `">` + anchor + inner + `</` + tag + `>`
	})
}

func extractTOC(htmlStr string) []TOCEntry {
	matches := headingRe.FindAllStringSubmatch(htmlStr, -1)
	toc := make([]TOCEntry, 0, len(matches))
	for _, m := range matches {
		level := 2
		if m[1] == "3" {
			level = 3
		}
		// Strip anchor-link element before extracting text
		inner := anchorLinkRe.ReplaceAllString(m[3], "")
		toc = append(toc, TOCEntry{
			Level: level,
			ID:    m[2],
			Text:  stripTags(inner),
		})
	}
	return toc
}

func extractTitle(htmlStr string) string {
	re := regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)
	if m := re.FindStringSubmatch(htmlStr); len(m) >= 2 {
		return stripTags(m[1])
	}
	return ""
}

func stripTags(s string) string {
	return strings.TrimSpace(tagRe.ReplaceAllString(s, ""))
}

// calloutRe matches a blockquote whose first paragraph starts with [!TYPE]
// Goldmark renders it as: <blockquote>\n<p>[!NOTE]\nrest</p>...
var calloutRe = regexp.MustCompile(`(?s)<blockquote>\s*<p>\[!(NOTE|TIP|WARNING|DANGER|INFO)\]\n?(.*?)</p>(.*?)</blockquote>`)

var calloutMeta = map[string][2]string{
	"NOTE":    {"💡", "callout-note"},
	"INFO":    {"ℹ️", "callout-info"},
	"TIP":     {"✅", "callout-tip"},
	"WARNING": {"⚠️", "callout-warning"},
	"DANGER":  {"🚨", "callout-danger"},
}

// transformCallouts converts GitHub-style blockquote callouts into styled divs.
// Syntax in Markdown:
//
//	> [!NOTE]
//	> This is a note.
func transformCallouts(htmlStr string) string {
	return calloutRe.ReplaceAllStringFunc(htmlStr, func(match string) string {
		subs := calloutRe.FindStringSubmatch(match)
		if len(subs) < 4 {
			return match
		}
		kind := subs[1]
		firstPara := strings.TrimSpace(subs[2])
		rest := strings.TrimSpace(subs[3])

		meta, ok := calloutMeta[kind]
		if !ok {
			return match
		}
		icon, class := meta[0], meta[1]
		label := strings.Title(strings.ToLower(kind))

		inner := ""
		if firstPara != "" {
			inner += "<p>" + firstPara + "</p>"
		}
		inner += rest

		return `<div class="callout ` + class + `">` +
			`<div class="callout-title">` + icon + ` ` + label + `</div>` +
			`<div class="callout-body">` + inner + `</div>` +
			`</div>`
	})
}

// ── API widget ─────────────────────────────────────────────────────────────

// apiFenceRe matches ```api ... ``` fenced code blocks.
var apiFenceRe = regexp.MustCompile("(?ms)^```api\\s*\n(.*?)\n```")

// apiFenceCounter is reset per Render call via replaceAPIWithPlaceholders.
// templateVarRe finds {{varName}} placeholders in URLs/headers.
var templateVarRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// replaceAPIWithPlaceholders replaces ```api fences with opaque tokens and
// stores the rendered HTML widget in blocks.
func replaceAPIWithPlaceholders(src []byte, blocks map[string]string) []byte {
	counter := 0
	return apiFenceRe.ReplaceAllFunc(src, func(match []byte) []byte {
		subs := apiFenceRe.FindSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		key := fmt.Sprintf("APIPLACEHOLDER%d", counter)
		counter++
		blocks[key] = renderAPIWidget(string(subs[1]))
		return []byte("<div>" + key + "</div>")
	})
}

// restorePlaceholders swaps generic placeholder tokens back to their HTML.
func restorePlaceholders(html string, blocks map[string]string) string {
	for key, html2 := range blocks {
		html = strings.ReplaceAll(html, "<div>"+key+"</div>", html2)
	}
	return html
}

// apiSpec holds a parsed ```api block.
type apiSpec struct {
	method  string
	url     string
	headers [][2]string // ordered key/value pairs
	vars    []string    // unique {{varName}} placeholders found in url+headers
}

// parseAPISpec parses the content of a ```api fence.
// First non-empty line: METHOD URL
// Remaining lines: Header-Name: value
func parseAPISpec(src string) apiSpec {
	var spec apiSpec
	lines := strings.Split(strings.TrimSpace(src), "\n")
	first := true
	seen := map[string]bool{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if first {
			first = false
			parts := strings.SplitN(line, " ", 2)
			spec.method = strings.ToUpper(parts[0])
			if len(parts) > 1 {
				spec.url = strings.TrimSpace(parts[1])
			}
			for _, m := range templateVarRe.FindAllStringSubmatch(spec.url, -1) {
				if !seen[m[1]] {
					seen[m[1]] = true
					spec.vars = append(spec.vars, m[1])
				}
			}
			continue
		}
		// Header line: Key: Value
		if idx := strings.Index(line, ":"); idx > 0 {
			k := strings.TrimSpace(line[:idx])
			v := strings.TrimSpace(line[idx+1:])
			spec.headers = append(spec.headers, [2]string{k, v})
			for _, m := range templateVarRe.FindAllStringSubmatch(v, -1) {
				if !seen[m[1]] {
					seen[m[1]] = true
					spec.vars = append(spec.vars, m[1])
				}
			}
		}
	}
	return spec
}

// methodColor returns a Tailwind-style badge class for an HTTP method.
func methodColor(method string) string {
	switch method {
	case "GET":
		return "api-method-get"
	case "POST":
		return "api-method-post"
	case "PUT":
		return "api-method-put"
	case "PATCH":
		return "api-method-patch"
	case "DELETE":
		return "api-method-delete"
	default:
		return "api-method-other"
	}
}

// renderAPIWidget converts a parsed ```api block to the HTML widget.
func renderAPIWidget(src string) string {
	spec := parseAPISpec(src)
	if spec.url == "" {
		return `<div class="callout callout-danger"><div class="callout-title">🚨 API block error</div><div class="callout-body"><p>Missing URL in api block.</p></div></div>`
	}

	// Unique widget ID for scoping inputs.
	widgetID := fmt.Sprintf("api-%d", widgetCounter)
	widgetCounter++

	var b strings.Builder

	b.WriteString(`<div class="api-widget" id="` + widgetID + `">`)

	// ── Title bar (always visible) ──────────────────────────────────────────
	b.WriteString(`<div class="api-titlebar">`)
	b.WriteString(`<span class="api-method ` + methodColor(spec.method) + `">` + htmlEscape(spec.method) + `</span>`)
	b.WriteString(`<span class="api-url">` + htmlEscape(spec.url) + `</span>`)
	b.WriteString(`<button class="api-toggle" onclick="apiToggle('` + widgetID + `')" aria-expanded="false">`)
	b.WriteString(`<span class="api-toggle-label">Try it</span>`)
	b.WriteString(`<svg class="api-chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg>`)
	b.WriteString(`</button>`)
	b.WriteString(`</div>`) // api-titlebar

	// ── Expandable panel ────────────────────────────────────────────────────
	b.WriteString(`<div class="api-panel" hidden>`)

	// Template variables ({{token}}, etc.)
	if len(spec.vars) > 0 {
		b.WriteString(`<div class="api-section">`)
		b.WriteString(`<div class="api-section-title">Variables</div>`)
		for _, v := range spec.vars {
			b.WriteString(`<div class="api-field-row">`)
			b.WriteString(`<label class="api-field-label">` + htmlEscape(v) + `</label>`)
			b.WriteString(`<input class="api-input" data-var="` + htmlEscape(v) + `" placeholder="` + htmlEscape(v) + `">`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}

	// Auth
	b.WriteString(`<div class="api-section">`)
	b.WriteString(`<div class="api-section-title">Auth</div>`)
	b.WriteString(`<div class="api-auth-tabs">`)
	b.WriteString(`<button class="api-auth-tab active" onclick="apiAuthTab('` + widgetID + `','none',this)">None</button>`)
	b.WriteString(`<button class="api-auth-tab" onclick="apiAuthTab('` + widgetID + `','bearer',this)">Bearer</button>`)
	b.WriteString(`<button class="api-auth-tab" onclick="apiAuthTab('` + widgetID + `','basic',this)">Basic</button>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="api-auth-panel" data-auth="bearer" style="display:none">`)
	b.WriteString(`<input class="api-input" data-role="bearer-token" placeholder="Bearer token">`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="api-auth-panel" data-auth="basic" style="display:none">`)
	b.WriteString(`<input class="api-input" data-role="basic-user" placeholder="Username" style="margin-bottom:0.4rem">`)
	b.WriteString(`<input class="api-input" data-role="basic-pass" placeholder="Password" type="password">`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`) // api-section auth

	// Static headers preview (if any defined in the block)
	if len(spec.headers) > 0 {
		b.WriteString(`<div class="api-section">`)
		b.WriteString(`<div class="api-section-title">Headers</div>`)
		for _, h := range spec.headers {
			b.WriteString(`<div class="api-field-row">`)
			b.WriteString(`<span class="api-field-label api-field-label--fixed">` + htmlEscape(h[0]) + `</span>`)
			b.WriteString(`<span class="api-field-value">` + htmlEscape(h[1]) + `</span>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}

	// Body (for POST/PUT/PATCH)
	if spec.method == "POST" || spec.method == "PUT" || spec.method == "PATCH" {
		b.WriteString(`<div class="api-section">`)
		b.WriteString(`<div class="api-section-title">Body <span class="api-hint">JSON</span></div>`)
		b.WriteString(`<textarea class="api-textarea" data-role="body" rows="4" placeholder='{"key": "value"}'></textarea>`)
		b.WriteString(`</div>`)
	}

	// Send button
	b.WriteString(`<div class="api-send-row">`)
	b.WriteString(`<button class="api-send-btn" onclick="apiSend('` + widgetID + `')">`)
	b.WriteString(`<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M5 12h14M12 5l7 7-7 7"/></svg> Send`)
	b.WriteString(`</button>`)
	b.WriteString(`<span class="api-status-badge" data-role="status"></span>`)
	b.WriteString(`</div>`)

	// Response area (hidden until response arrives)
	b.WriteString(`<div class="api-response" data-role="response" style="display:none">`)
	b.WriteString(`<div class="api-response-tabs">`)
	b.WriteString(`<button class="api-resp-tab active" onclick="apiRespTab('` + widgetID + `','body',this)">Body</button>`)
	b.WriteString(`<button class="api-resp-tab" onclick="apiRespTab('` + widgetID + `','headers',this)">Headers</button>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="api-resp-panel" data-resp="body"><pre class="api-resp-pre" data-role="resp-body"></pre></div>`)
	b.WriteString(`<div class="api-resp-panel" data-resp="headers" style="display:none"><table class="api-resp-headers" data-role="resp-headers"></table></div>`)
	b.WriteString(`</div>`) // api-response

	b.WriteString(`</div>`) // api-panel
	b.WriteString(`</div>`) // api-widget

	// Embed widget metadata as JSON for JS to use (method, url, headers)
	headersJSON := "["
	for i, h := range spec.headers {
		if i > 0 {
			headersJSON += ","
		}
		headersJSON += `["` + jsEscape(h[0]) + `","` + jsEscape(h[1]) + `"]`
	}
	headersJSON += "]"

	b.WriteString(`<script>window.__apiWidgets=window.__apiWidgets||{};window.__apiWidgets["` + widgetID + `"]=`)
	b.WriteString(`{"method":"` + jsEscape(spec.method) + `","url":"` + jsEscape(spec.url) + `","headers":` + headersJSON + `}`)
	b.WriteString(`;` + `</script>`)

	return b.String()
}

// widgetCounter generates unique IDs across a single Render call.
// Not goroutine-safe but Render is called per-request so this is fine.
var widgetCounter int

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}

func jsEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", ``)
	return s
}
