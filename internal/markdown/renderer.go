package markdown

import (
	"bytes"
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

	// Pre-process: replace ```mermaid blocks with raw HTML divs before goldmark
	// sees them, so chroma never gets a chance to syntax-highlight them.
	preprocessed := extractMermaidBlocks(body)

	var buf bytes.Buffer
	if err := r.md.Convert(preprocessed, &buf); err != nil {
		return nil, err
	}

	htmlStr := transformCallouts(addAnchorLinks(buf.String()))
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
