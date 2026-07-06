package markdown

import (
	"fmt"
	"regexp"
	"strings"

	nethtml "golang.org/x/net/html"
)

type mkDocsBlock struct {
	Type    string
	Title   string
	Content string
	Details bool
	Open    bool
}

type mkDocsBlockStyle struct {
	Label string
	Icon  string
	Class string
}

var (
	mkDocsBlockStartRe  = regexp.MustCompile(`^(!!!|\?\?\?)(\+?)\s+([A-Za-z0-9_-]+)(?:\s+"(.*)")?\s*$`)
	mkDocsSuperscriptRe = regexp.MustCompile(`\^([A-Za-z0-9][A-Za-z0-9._-]*)\^`)

	mkDocsBlockStyles = map[string]mkDocsBlockStyle{
		"note":    {Label: "Note", Icon: "💡", Class: "callout-note"},
		"info":    {Label: "Info", Icon: "ℹ️", Class: "callout-info"},
		"tip":     {Label: "Tip", Icon: "✓", Class: "callout-tip"},
		"success": {Label: "Success", Icon: "✓", Class: "callout-tip"},
		"warning": {Label: "Warning", Icon: "⚠", Class: "callout-warning"},
		"todo":    {Label: "Todo", Icon: "⚠", Class: "callout-warning"},
		"danger":  {Label: "Danger", Icon: "✕", Class: "callout-danger"},
		"failure": {Label: "Failure", Icon: "✕", Class: "callout-danger"},
		"cite":    {Label: "Quote", Icon: "❝", Class: "callout-note"},
		"tldr":    {Label: "Summary", Icon: "ℹ️", Class: "callout-info"},
	}

	mkDocsEmoji = map[string]string{
		":star:":               "⭐",
		":construction:":       "🚧",
		":warning:":            "⚠️",
		":small_red_triangle:": "🔺",
		":motorway:":           "🛣️",
		":key:":                "🔑",
		":cop:":                "👮",
		":postgresql:":         "🐘",
	}
)

// replaceMkDocsBlocksWithPlaceholders extracts top-level MkDocs admonitions.
// Nested blocks remain in the dedented content and are extracted recursively
// when restoreMkDocsBlocks renders that content.
func replaceMkDocsBlocksWithPlaceholders(src []byte, blocks map[string]mkDocsBlock) []byte {
	lines := strings.SplitAfter(string(src), "\n")
	var out strings.Builder
	var fenceMarker string
	var fenceLength int

	for i := 0; i < len(lines); {
		line := strings.TrimRight(lines[i], "\r\n")
		if fenceMarker != "" {
			out.WriteString(lines[i])
			if isClosingFence(line, fenceMarker, fenceLength) {
				fenceMarker = ""
				fenceLength = 0
			}
			i++
			continue
		}
		if subs := apiFenceLineRe.FindStringSubmatch(line); len(subs) == 3 {
			fenceMarker = string(subs[1][0])
			fenceLength = len(subs[1])
			out.WriteString(lines[i])
			i++
			continue
		}

		match := mkDocsBlockStartRe.FindStringSubmatch(line)
		if len(match) == 0 {
			out.WriteString(lines[i])
			i++
			continue
		}

		var content strings.Builder
		j := i + 1
		for ; j < len(lines); j++ {
			bodyLine := strings.TrimRight(lines[j], "\r\n")
			if strings.TrimSpace(bodyLine) == "" {
				content.WriteString(lineEnding(lines[j]))
				continue
			}
			if !hasMkDocsIndent(bodyLine) {
				break
			}
			content.WriteString(removeMkDocsIndent(lines[j]))
		}

		key := fmt.Sprintf("DYNOMKDOCSPLACEHOLDER%d", len(blocks))
		blocks[key] = mkDocsBlock{
			Type:    strings.ToLower(match[3]),
			Title:   match[4],
			Content: strings.TrimRight(content.String(), "\r\n"),
			Details: match[1] == "???",
			Open:    match[1] == "???" && match[2] == "+",
		}
		out.WriteString("<div>" + key + "</div>\n")
		i = j
	}

	return []byte(out.String())
}

func hasMkDocsIndent(line string) bool {
	return strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ")
}

func removeMkDocsIndent(line string) string {
	if strings.HasPrefix(line, "\t") {
		return strings.TrimPrefix(line, "\t")
	}
	return strings.TrimPrefix(line, "    ")
}

func lineEnding(line string) string {
	if strings.HasSuffix(line, "\r\n") {
		return "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return "\n"
	}
	return ""
}

func (r *Renderer) restoreMkDocsBlocks(htmlStr string, blocks map[string]mkDocsBlock, opts RenderOptions) (string, error) {
	for key, block := range blocks {
		bodyHTML, err := r.renderMarkdownBody([]byte(block.Content), opts)
		if err != nil {
			return "", fmt.Errorf("render MkDocs %s block: %w", block.Type, err)
		}
		titleHTML, err := r.renderMkDocsTitle(block)
		if err != nil {
			return "", fmt.Errorf("render MkDocs %s title: %w", block.Type, err)
		}
		rendered := renderMkDocsBlock(block, titleHTML, bodyHTML)
		htmlStr = strings.ReplaceAll(htmlStr, "<div>"+key+"</div>", rendered)
	}
	return htmlStr, nil
}

func (r *Renderer) renderMkDocsTitle(block mkDocsBlock) (string, error) {
	style := mkDocsStyle(block.Type)
	title := block.Title
	if title == "" {
		title = style.Label
	}
	var out strings.Builder
	if err := r.md.Convert([]byte(title), &out); err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(out.String()), "<p>"), "</p>"), nil
}

func renderMkDocsBlock(block mkDocsBlock, titleHTML, bodyHTML string) string {
	style := mkDocsStyle(block.Type)
	classes := "callout " + style.Class + " mkdocs-admonition mkdocs-admonition-" + block.Type
	title := `<span class="mkdocs-admonition-icon" aria-hidden="true">` + style.Icon + `</span><span>` + titleHTML + `</span>`
	if !block.Details {
		rendered := `<div class="` + classes + `"><div class="callout-title mkdocs-admonition-title">` + title + `</div>`
		if strings.TrimSpace(bodyHTML) != "" {
			rendered += `<div class="callout-body mkdocs-admonition-body">` + bodyHTML + `</div>`
		}
		return rendered + `</div>`
	}

	open := ""
	if block.Open {
		open = " open"
	}
	return `<details class="` + classes + ` mkdocs-details"` + open + `>` +
		`<summary class="callout-title mkdocs-admonition-title">` + title +
		`<svg class="mkdocs-details-chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg></summary>` +
		`<div class="callout-body mkdocs-admonition-body">` + bodyHTML + `</div></details>`
}

func mkDocsStyle(kind string) mkDocsBlockStyle {
	if style, ok := mkDocsBlockStyles[kind]; ok {
		return style
	}
	return mkDocsBlockStyle{Label: strings.Title(kind), Icon: "💡", Class: "callout-note"}
}

// transformMkDocsInlineHTML applies the small PyMdown inline subset after
// Goldmark has generated heading IDs. HTML tokenization keeps code, scripts,
// styles and textareas byte-for-byte unchanged.
func transformMkDocsInlineHTML(htmlStr string) string {
	tokenizer := nethtml.NewTokenizer(strings.NewReader(htmlStr))
	var out strings.Builder
	skipDepth := 0
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case nethtml.ErrorToken:
			return out.String()
		case nethtml.StartTagToken:
			token := tokenizer.Token()
			if skipDepth > 0 {
				skipDepth++
			} else if isMkDocsInlineSkipElement(token.Data) {
				skipDepth = 1
			}
			out.Write(tokenizer.Raw())
		case nethtml.EndTagToken:
			out.Write(tokenizer.Raw())
			if skipDepth > 0 {
				skipDepth--
			}
		case nethtml.TextToken:
			if skipDepth > 0 {
				out.Write(tokenizer.Raw())
			} else {
				out.WriteString(transformMkDocsText(string(tokenizer.Raw())))
			}
		default:
			out.Write(tokenizer.Raw())
		}
	}
}

func isMkDocsInlineSkipElement(name string) bool {
	switch name {
	case "code", "pre", "script", "style", "textarea":
		return true
	default:
		return false
	}
}

func transformMkDocsText(text string) string {
	text = mkDocsSuperscriptRe.ReplaceAllString(text, `<sup>$1</sup>`)
	for shortcode, emoji := range mkDocsEmoji {
		text = strings.ReplaceAll(text, shortcode, emoji)
	}
	return text
}
