package markdown

import (
	"fmt"
	"html"
	"strings"
	"unicode"

	xhtml "golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

// Keep Goldmark's existing IDs (including search and TOC links), while accepting
// Unicode heading links commonly used by Markdown repositories such as GitHub.
func addUnicodeHeadingAnchors(fragment string) string {
	used := make(map[string]bool)
	tokens := xhtml.NewTokenizer(strings.NewReader(fragment))
	for {
		kind := tokens.Next()
		if kind == xhtml.ErrorToken {
			break
		}
		if kind != xhtml.StartTagToken && kind != xhtml.SelfClosingTagToken {
			continue
		}
		for _, attr := range tokens.Token().Attr {
			if attr.Key == "id" || attr.Key == "name" {
				used[attr.Val] = true
			}
		}
	}
	counts := make(map[string]int)
	return allHeadingRe.ReplaceAllStringFunc(fragment, func(heading string) string {
		parts := allHeadingRe.FindStringSubmatch(heading)
		label := html.UnescapeString(stripTags(anchorLinkRe.ReplaceAllString(parts[3], "")))
		base := unicodeHeadingSlug(label)
		if base == "" {
			return heading
		}
		id := base
		if count := counts[base]; count > 0 {
			id = fmt.Sprintf("%s-%d", base, count)
		}
		counts[base]++
		if used[id] {
			return heading
		}
		used[id] = true
		alias := `<span id="` + html.EscapeString(id) + `" class="dyno-heading-alias" aria-hidden="true"></span>`
		return `<` + parts[1] + ` id="` + parts[2] + `">` + alias + parts[3] + `</` + parts[1] + `>`
	})
}

func unicodeHeadingSlug(label string) string {
	var slug strings.Builder
	for _, r := range strings.ToLower(norm.NFC.String(strings.TrimSpace(label))) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), unicode.IsMark(r), r == '-', r == '_':
			slug.WriteRune(r)
		case unicode.IsSpace(r):
			slug.WriteByte('-')
		}
	}
	return slug.String()
}
