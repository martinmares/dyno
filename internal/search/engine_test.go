package search

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

func TestRelevanceAndSpelling(t *testing.T) {
	idx := New()
	idx.Add(Document{Path: "/mention", Title: "General notes", Body: strings.Repeat("Configuration details. ", 100) + "PostgreSQL database permissions."})
	idx.Add(Document{Path: "/guide", Title: "PostgreSQL permissions", Body: "Database configuration and migration."})
	idx.Add(Document{Path: "/other", Title: "Database overview", Body: "Unrelated configuration."})
	for _, input := range []string{"postgresql", "postgres", "postgreql", "POSTGRESQL", "postgresql permissions"} {
		hits := idx.Search(input)
		if len(hits) == 0 || hits[0].Path != "/guide" {
			t.Fatalf("%q: title should win: %+v", input, hits)
		}
		if hits[0].Approximate != (input == "postgreql") {
			t.Fatalf("%q: wrong spelling indicator: %+v", input, hits[0])
		}
		if input == "postgreql" && !strings.Contains(hits[0].Title, "<mark>PostgreSQL</mark>") {
			t.Fatalf("misspelling must highlight matched word: %+v", hits[0])
		}
	}
	if got := idx.Search("postgresql unknownword"); len(got) == 0 || !got[0].Partial {
		t.Fatalf("partial fallback not labelled: %+v", got)
	}
	if got := idx.Query("postgresql unknownword", Options{}, nil); len(got) != 0 {
		t.Fatalf("explicit all must not relax: %+v", got)
	}
	if got := idx.Search(`"PostgreQL permissions"`); len(got) != 0 {
		t.Fatalf("quoted phrase must not be corrected: %+v", got)
	}
	if got := idx.Query("postgreql", Options{OmitSnippets: true}, nil); len(got) == 0 || !got[0].Approximate {
		t.Fatalf("Explorer must also tolerate typos: %+v", got)
	}
}

func TestTechnicalIdentifiers(t *testing.T) {
	idx := New()
	idx.Add(Document{Path: "/correct", Title: "REQ0129276", Body: "Use tsm-pd-backend on release/RE63 with version 2.4."})
	idx.Add(Document{Path: "/neighbor", Title: "REQ0129277", Body: "Use tsm-sa-backend on release/RE64 with version 2.5."})
	for _, input := range []string{"REQ0129276", "req012927", "tsm-pd-backend", "release/RE63", "2.4"} {
		hits := idx.Search(input)
		if len(hits) == 0 || hits[0].Path != "/correct" || hits[0].Approximate {
			t.Fatalf("identifier %q: %+v", input, hits)
		}
	}
	for _, input := range []string{"REQ0129278", "tsm-pd-backenx", "release/RE65", "2.6"} {
		if got := idx.Search(input); len(got) != 0 {
			t.Fatalf("identifier must not be typo corrected %q: %+v", input, got)
		}
	}
	idx.Add(Document{Path: "/guides/req0129276-access", Title: "Guide", Body: "Access rules."})
	if hits := idx.Query("REQ0129276", Options{Scope: "path"}, nil); len(hits) != 1 || hits[0].Path != "/guides/req0129276-access" {
		t.Fatalf("path scope lost basename identifiers: %+v", hits)
	}
	if hits := idx.Query("rules.", Options{Scope: "body"}, nil); len(hits) != 1 {
		t.Fatalf("sentence punctuation broke word matching: %+v", hits)
	}
}

func TestSectionsMatchReaderAnchorsAndIgnoreFrontmatter(t *testing.T) {
	source := []byte("---\ntitle: Secret metadata\n---\n# Guide\n\nIntro\n\n## Přístupy\n\nUnrelated text.\n\n### Migrace {#migration}\n\nPostgreSQL migration instructions.\n\n## Přístupy\n\nSecond chapter.\n\n```md\n## Not a heading\n```\n")
	sections := extractSections(source)
	var anchors []string
	for _, section := range sections {
		if section.Anchor != "" {
			anchors = append(anchors, section.Anchor)
		}
		if strings.Contains(section.Body, "Secret metadata") {
			t.Fatal("indexed frontmatter")
		}
	}
	md := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithParserOptions(parser.WithAutoHeadingID(), parser.WithAttribute()))
	var rendered strings.Builder
	if err := md.Convert(headingFrontmatter.ReplaceAll(source, nil), &rendered); err != nil {
		t.Fatal(err)
	}
	for _, anchor := range anchors {
		if !strings.Contains(rendered.String(), `id="`+anchor+`"`) {
			t.Fatalf("anchor %q not in reader HTML: %s", anchor, rendered.String())
		}
	}
	if len(anchors) != 4 {
		t.Fatalf("code heading should not create section: %+v", sections)
	}
	idx := New()
	idx.Add(Document{Path: "/guide", Title: "Guide", Body: "PostgreSQL migration instructions.", Sections: sections})
	hits := idx.Search("postgresql migration")
	if len(hits) != 1 || hits[0].Anchor != "migration" || !strings.Contains(hits[0].Section, "Přístupy / Migrace") || !strings.Contains(hits[0].Snippet, "<mark>PostgreSQL</mark>") {
		t.Fatalf("missing chapter context: %+v", hits)
	}
}

func TestIndexReplacementAndFilteredPagination(t *testing.T) {
	idx := New()
	for i := range 75 {
		idx.Add(Document{Path: fmt.Sprintf("/item-%03d", i), Title: "Common title", Body: "Common content."})
	}
	page, total := idx.SearchPage("common", nil, 20, 20)
	if total != 75 || len(page) != 20 || page[0].Path != "/item-020" {
		t.Fatalf("page: total=%d results=%+v", total, page)
	}
	page, total = idx.SearchPage("common", nil, 100, 20)
	if total != 75 || len(page) != 0 {
		t.Fatalf("out of range page: total=%d results=%+v", total, page)
	}
	page, total = idx.SearchPage("common", map[string]bool{"/item-074": true}, 0, 20)
	if total != 1 || len(page) != 1 || page[0].Path != "/item-074" {
		t.Fatalf("filter must precede limit: total=%d results=%+v", total, page)
	}
	idx.Add(Document{Path: "/item-074", Title: "Replacement", Body: "New text."})
	if idx.DocCount() != 75 || len(idx.SearchFiltered("common", map[string]bool{"/item-074": true})) != 0 || len(idx.Search("replacement")) != 1 {
		t.Fatal("document update left stale search entries")
	}
}

func TestPhraseRelevanceAndHeadingCoverage(t *testing.T) {
	idx := New()
	idx.Add(Document{Path: "/a-scattered", Title: "Guide", Body: "Migration of PostgreSQL and other runtime database permissions."})
	idx.Add(Document{Path: "/z-adjacent", Title: "Guide", Body: "PostgreSQL migration and other runtime database permissions."})
	if hits := idx.Query("postgresql migration", Options{}, nil); len(hits) != 2 || hits[0].Path != "/z-adjacent" {
		t.Fatalf("adjacent words must rank higher: %+v", hits)
	}
	idx.Add(Document{Path: "/heading", Title: "Guide", Sections: []Section{{Heading: "Přístupová oprávnění", Title: "Guide / Přístupová oprávnění", Anchor: "permissions", Body: "Details."}}})
	if hits := idx.Search("pristupova opravneni"); len(hits) != 1 || hits[0].Anchor != "permissions" {
		t.Fatalf("all headings must be searchable: %+v", hits)
	}
	if hits := idx.Search("."); len(hits) != 0 {
		t.Fatalf("punctuation must not return all documents: %+v", hits)
	}
}
