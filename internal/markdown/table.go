package markdown

import (
	"io"
	"strings"

	nethtml "golang.org/x/net/html"
)

// wrapMarkdownTables adds the interactive table shell around tables produced
// by Goldmark. The HTML tokenizer preserves the original table markup instead
// of reparsing or rebuilding user content.
func wrapMarkdownTables(htmlStr string, options TableOptions) string {
	if !options.Sortable && !options.Filter {
		return htmlStr
	}

	tokenizer := nethtml.NewTokenizer(strings.NewReader(htmlStr))
	var out strings.Builder
	var table strings.Builder
	tableDepth := 0
	hasHead := false
	hasBody := false

	flushTable := func() {
		tableHTML := table.String()
		if hasHead && hasBody {
			out.WriteString(renderTableWidget(tableHTML, options))
		} else {
			out.WriteString(tableHTML)
		}
		table.Reset()
		hasHead = false
		hasBody = false
	}

	for {
		tokenType := tokenizer.Next()
		if tokenType == nethtml.ErrorToken {
			if tokenizer.Err() != io.EOF && tableDepth == 0 {
				out.Write(tokenizer.Raw())
			}
			if tableDepth > 0 {
				flushTable()
			}
			return out.String()
		}

		var token nethtml.Token
		if tokenType == nethtml.StartTagToken || tokenType == nethtml.EndTagToken {
			token = tokenizer.Token()
		}
		raw := tokenizer.Raw()

		if tableDepth == 0 {
			if tokenType == nethtml.StartTagToken && token.Data == "table" {
				tableDepth = 1
				table.Write(raw)
				continue
			}
			out.Write(raw)
			continue
		}

		table.Write(raw)
		if tokenType == nethtml.StartTagToken {
			switch token.Data {
			case "table":
				tableDepth++
			case "thead":
				hasHead = true
			case "tbody":
				hasBody = true
			}
		} else if tokenType == nethtml.EndTagToken && token.Data == "table" {
			tableDepth--
			if tableDepth == 0 {
				flushTable()
			}
		}
	}
}

func renderTableWidget(tableHTML string, options TableOptions) string {
	var out strings.Builder
	out.WriteString(`<div class="dyno-table-widget" data-table-sortable="`)
	out.WriteString(boolString(options.Sortable))
	out.WriteString(`" data-table-filter="`)
	out.WriteString(boolString(options.Filter))
	out.WriteString(`">`)

	if options.Filter {
		out.WriteString(`<div class="dyno-table-toolbar">`)
		out.WriteString(`<label class="dyno-table-filter-control">`)
		out.WriteString(`<svg class="dyno-table-filter-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m21 21-4.35-4.35m2.35-5.65a8 8 0 1 1-16 0 8 8 0 0 1 16 0Z"/></svg>`)
		out.WriteString(`<input type="search" class="form-control form-control-sm dyno-table-filter" placeholder="Filter table" aria-label="Filter table" autocomplete="off">`)
		out.WriteString(`</label>`)
		out.WriteString(`<span class="dyno-table-status" aria-live="polite"></span>`)
		out.WriteString(`</div>`)
	}

	out.WriteString(`<div class="table-responsive dyno-table-responsive">`)
	out.WriteString(tableHTML)
	out.WriteString(`</div>`)
	if options.Filter {
		out.WriteString(`<div class="dyno-table-empty" hidden>No matching rows.</div>`)
	}
	out.WriteString(`</div>`)
	return out.String()
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
