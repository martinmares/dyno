package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

func reloadExplorerTest(t *testing.T, s *Server) {
	t.Helper()
	nav, err := navigation.BuildTree(s.contentDir, s.basePath)
	if err != nil {
		t.Fatal(err)
	}
	s.Reload(nav, search.New())
}
func explorerGET(t *testing.T, s *Server, target string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
	if rec.Code != 200 {
		t.Fatalf("%s: %d %s", target, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}
func TestExplorerOptInAndReader(t *testing.T) {
	ordinary := newTestServer(t)
	rec := httptest.NewRecorder()
	ordinary.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "dyno-explorer-body") {
		t.Fatal("default reader changed")
	}
	rec = httptest.NewRecorder()
	ordinary.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/_explore", nil))
	if rec.Code != 404 {
		t.Fatal("explorer exposed without opt-in")
	}
	s := newTestServer(t, func(c *Config) { c.Explore = true })
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Location") != "" || strings.Contains(rec.Body.String(), "dyno-explorer-body") {
		t.Fatalf("Explorer must not replace the home document: %d", rec.Code)
	}
	body := explorerGET(t, s, "/_explore")
	if !strings.Contains(body, "dyno-explorer-body") || !strings.Contains(body, "index.md") {
		t.Fatal("missing file browser")
	}
	body = explorerGET(t, s, "/?explore="+url.QueryEscape("q=permissions&tab=overview"))
	if !strings.Contains(body, `/_explore?q=permissions&amp;tab=overview`) {
		t.Fatal("reader lost explorer return state")
	}
}
func TestExplorerReaderFiltersStayInReader(t *testing.T) {
	for _, base := range []string{"", "/manual"} {
		t.Run(base, func(t *testing.T) {
			s := newTestServer(t, func(c *Config) {
				c.Explore = true
				c.SiteCfg.BasePath = &base
			})
			s.siteCfg.Frontmatter = config.FrontmatterConfig{Fields: map[string]config.FrontmatterFieldConfig{"owner": {Type: "text", Filterable: true}}}
			writeTestFile(t, filepath.Join(s.contentDir, "index.md"), "---\nowner: Merhaut\n---\n# Home\n\nReader content.\n")
			reloadExplorerTest(t, s)
			for _, query := range []string{"", "?meta.owner=Merhaut"} {
				body := explorerGET(t, s, base+"/"+query)
				if !strings.Contains(body, "Reader content.") || strings.Contains(body, "dyno-explorer-body") {
					t.Fatal("home must remain the reader, with or without filters")
				}
			}
			for _, query := range []string{"", "?meta.owner=Merhaut", "?read=1&meta.owner=Merhaut"} {
				req := httptest.NewRequest(http.MethodGet, base+"/"+query, nil)
				req.Header.Set("HX-Request", "true")
				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, req)
				if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Reader content.") || strings.Contains(rec.Body.String(), "dyno-explorer-body") {
					t.Fatalf("reader fragment replaced by Explorer: %d %s", rec.Code, rec.Body.String())
				}
				pushed, err := url.Parse(rec.Header().Get("HX-Push-Url"))
				if err != nil || pushed.Path != base+"/" {
					t.Fatalf("reader URL does not survive reload: %s", rec.Header().Get("HX-Push-Url"))
				}
				if strings.Contains(query, "meta.owner") && pushed.Query().Get("meta.owner") != "Merhaut" {
					t.Fatal("filter was lost")
				}
			}
			target := base + "/_explore?meta.owner=Merhaut"
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.Header.Set("HX-Request", "true")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || rec.Header().Get("HX-Redirect") != target || rec.Body.Len() != 0 {
				t.Fatal("Explorer shell must not be returned as a reader fragment")
			}
		})
	}
}

func TestExplorerFiltersColumnsSelectionAndRelations(t *testing.T) {
	s := newTestServer(t, func(c *Config) { c.Explore = true })
	s.siteCfg.Frontmatter = config.FrontmatterConfig{Display: []string{"team"}, Fields: map[string]config.FrontmatterFieldConfig{"team": {Label: "Custom team", Type: "text", Filterable: true}}}
	writeTestFile(t, filepath.Join(s.contentDir, "alpha", "one.md"), "---\nteam: Orange\n---\n# Alpha document\n\nA detailed introduction describing alpha permissions.\n\n[Two](../beta/two.md#details)\n")
	writeTestFile(t, filepath.Join(s.contentDir, "beta", "two.md"), "---\nteam: Blue\n---\n# Beta document\n\nA detailed introduction describing beta permissions.\n")
	reloadExplorerTest(t, s)
	body := explorerGET(t, s, "/_explore?meta.team=Orange&columns=team&columns_set=1&selected=/alpha/one")
	if !strings.Contains(body, "Custom team") || !strings.Contains(body, "Document preview") || !strings.Contains(body, "alpha/one.md") {
		t.Fatal("missing configured fields or preview")
	}
	if strings.Contains(body, "beta/two.md") {
		t.Fatal("metadata filter leaked another row")
	}
	catalog, err := s.getExplorerCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Documents["/alpha/one"].Outgoing) != 1 || len(catalog.Documents["/beta/two"].Incoming) != 1 {
		t.Fatal("missing actual Markdown relationships")
	}
	body = explorerGET(t, s, "/_explore?tab=relations")
	if !strings.Contains(body, "alpha -&gt; beta") {
		t.Fatal("missing folder-level connection")
	}
	body = explorerGET(t, s, "/_explore?tab=overview&folder=alpha")
	if !strings.Contains(body, "Unfilled metadata") || !strings.Contains(body, "Custom team") {
		t.Fatal("missing generic overview")
	}
	body = explorerGET(t, s, "/_explore?q=permissions&scope=body&folder=alpha")
	if strings.Contains(body, "beta/two.md") || !strings.Contains(body, "alpha/one.md") {
		t.Fatal("folder/full-text intersection failed")
	}
}
func TestExplorerPaginatesWithoutQuickSearchLimit(t *testing.T) {
	s := newTestServer(t, func(c *Config) { c.Explore = true })
	for i := range 66 {
		writeTestFile(t, filepath.Join(s.contentDir, "many", fmt.Sprintf("item-%03d.md", i)), fmt.Sprintf("# Common item %03d\n\nShared content.\n", i))
	}
	reloadExplorerTest(t, s)
	first := explorerGET(t, s, "/_explore?q=common")
	second := explorerGET(t, s, "/_explore?q=common&page=2")
	if strings.Count(first, "data-explorer-select") != 50 || strings.Count(second, "data-explorer-select") != 16 {
		t.Fatal("incorrect pagination")
	}
	extreme := explorerGET(t, s, "/_explore?q=common&page=999999")
	if !strings.Contains(extreme, "Page 2 / 2") {
		t.Fatal("page not clamped")
	}
}

func TestExplorerDateAndMissingFiltersAreConfigDriven(t *testing.T) {
	s := newTestServer(t, func(c *Config) { c.Explore = true })
	s.siteCfg.Frontmatter = config.FrontmatterConfig{Fields: map[string]config.FrontmatterFieldConfig{"review": {Type: "date", Label: "Review date"}, "category": {Type: "text"}}}
	writeTestFile(t, filepath.Join(s.contentDir, "dated.md"), "---\nreview: 2026-10-16\ncategory: defined\n---\n# Dated\n")
	writeTestFile(t, filepath.Join(s.contentDir, "future.md"), "---\nreview: 2026-11-01\n---\n# Future\n")
	reloadExplorerTest(t, s)
	body := explorerGET(t, s, "/_explore?date_field=review&after=2026-10-16&before=2026-10-16")
	if strings.Count(body, "data-explorer-select") != 1 || !strings.Contains(body, "dated.md") {
		t.Fatal("inclusive date range failed")
	}
	body = explorerGET(t, s, "/_explore?missing=category&date_field=review&after=2026-10-01")
	if strings.Count(body, "data-explorer-select") != 1 || !strings.Contains(body, "future.md") {
		t.Fatal("missing field/date intersection failed")
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/_explore?date_field=review&after=invalid", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatal("invalid date accepted")
	}
}

func TestExplorerHonoursBasePath(t *testing.T) {
	s := newTestServer(t, func(c *Config) { c.Explore = true; base := "/manual"; c.SiteCfg.BasePath = &base })
	reloadExplorerTest(t, s)
	body := explorerGET(t, s, "/manual/_explore?selected=/manual/")
	if !strings.Contains(body, `action="/manual/_explore"`) || !strings.Contains(body, `/manual/?explore=`) {
		t.Fatal("explorer or reader URL lost base path")
	}
	explorerGET(t, s, "/manual/")
}
func TestExplorerDoesNotExposeExcludedDocumentsOrPaths(t *testing.T) {
	s := newTestServer(t, func(c *Config) { c.Explore = true })
	writeTestFile(t, filepath.Join(s.contentDir, "private.md"), "# Private\n")
	// The catalog is built only from the published navigation, not a new disk walk.
	body := explorerGET(t, s, "/_explore?selected=../../private.md")
	if strings.Contains(body, "private.md") || strings.Contains(body, s.contentDir) {
		t.Fatal("unpublished path leaked")
	}
	catalog, err := s.getExplorerCatalog()
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	err = s.writeDocument(filepath.Join(s.contentDir, "index.md"), []byte("# Updated home\n\nA new meaningful overview of permissions.\n"), 0o644)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := s.getExplorerCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if catalog == fresh {
		t.Fatal("editor save did not invalidate catalog")
	}
	if len(fresh.Index.Query("meaningful", search.Options{}, nil)) != 1 {
		t.Fatal("saved content not searchable")
	}
}
