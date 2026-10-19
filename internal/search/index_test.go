package search

import (
	"fmt"
	"strings"
	"testing"
)

func TestQueryPhrasesAccentsScopesAndMatches(t *testing.T) {
	idx := New()
	idx.Add(Document{Path: "/guides/database", Title: "Přístupová oprávnění", Body: "An overview of database configuration and runtime permissions."})
	idx.Add(Document{Path: "/reference/runtime", Title: "Runtime setup", Body: "Database permissions and configuration are described here."})
	for _, tt := range []struct {
		query   string
		options Options
		count   int
	}{
		{"pristupova opravneni", Options{}, 1},
		{"PŘÍSTUPOVÁ", Options{}, 1},
		{"pri\u0301stupova\u0301", Options{}, 1},
		{`"database configuration"`, Options{}, 1},
		{`"permissions and configuration"`, Options{}, 1},
		{"database runtime", Options{}, 2},
		{"database absent", Options{}, 0},
		{"database absent", Options{Match: "any"}, 2},
		{"permissions", Options{Scope: "title"}, 0},
		{"runtime", Options{Scope: "title"}, 1},
		{"guides", Options{Scope: "path"}, 1},
		{"pristupova", Options{Scope: "body"}, 0},
	} {
		t.Run(tt.query+tt.options.Scope+tt.options.Match, func(t *testing.T) {
			if got := idx.Query(tt.query, tt.options, nil); len(got) != tt.count {
				t.Fatalf("got %d results, want %d: %+v", len(got), tt.count, got)
			}
		})
	}
}

func TestQueryHighlightsSafelyAndBoostsTitles(t *testing.T) {
	idx := New()
	idx.Add(Document{Path: "/body", Title: "Generic guide", Body: "Database database database."})
	idx.Add(Document{Path: "/title", Title: `Databáze <script>alert("x")</script>`, Body: "A description."})
	hits := idx.Query("databaze", Options{}, nil)
	if len(hits) != 1 || !strings.Contains(hits[0].Title, "<mark>Databáze</mark>") || strings.Contains(hits[0].Title, "<script>") {
		t.Fatalf("unsafe or missing highlight: %+v", hits)
	}
	if got := highlightTerms("A & B < C", []string{"b"}); got != "A &amp; <mark>B</mark> &lt; C" {
		t.Fatalf("unsafe HTML: %s", got)
	}
	idx.Add(Document{Path: "/boost", Title: "Database", Body: "Short."})
	if hits = idx.Search("database"); hits[0].Path != "/boost" {
		t.Fatalf("title not boosted: %+v", hits)
	}
}

func TestQueryPaginationBoundaryAndAllowedDocuments(t *testing.T) {
	idx := New()
	for i := range 75 {
		idx.Add(Document{Path: fmt.Sprintf("/item-%03d", i), Title: "Common title", Body: "Common content."})
	}
	if got := len(idx.Search("common")); got != 20 {
		t.Fatalf("quick search limit: %d", got)
	}
	if got := len(idx.Query("common", Options{OmitSnippets: true}, nil)); got != 75 {
		t.Fatalf("explorer must not truncate: %d", got)
	}
	hits := idx.Query("common", Options{}, map[string]bool{"/item-025": true})
	if len(hits) != 1 || hits[0].Path != "/item-025" {
		t.Fatalf("filter not respected: %+v", hits)
	}
	if got := len(idx.Query("", Options{}, map[string]bool{})); got != 0 {
		t.Fatalf("empty allowed set leaked documents: %d", got)
	}
	if got := idx.Query("common", Options{Limit: 2}, nil); len(got) != 2 || got[0].Path != "/item-000" {
		t.Fatalf("unstable tie ordering: %+v", got)
	}
}

func TestHeadingsAreMarkdownHeadingsNotCodeOrFrontmatter(t *testing.T) {
	got := extractHeadings("---\ntitle: Example\n---\n# Title\n\n```md\n## Not a chapter\n```\n\n## Real chapter\n\nSetext chapter\n---\n\n### Details\n")
	if strings.Join(got, "|") != "Real chapter|Setext chapter|Details" {
		t.Fatalf("incorrect preview headings: %+v", got)
	}
}

func BenchmarkExplorerQuery(b *testing.B) {
	idx := New()
	for i := range 1000 {
		idx.Add(Document{Path: fmt.Sprintf("/docs/%d", i), Title: "Platform guide", Body: strings.Repeat("Technical documentation and configuration. ", 200)})
	}
	b.ResetTimer()
	for b.Loop() {
		idx.Query("configuration", Options{OmitSnippets: true}, nil)
	}
}
