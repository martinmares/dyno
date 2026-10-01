package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/browse"
	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/search"
)

func newBrowseTestServer(t *testing.T, paths ...string) *Server {
	t.Helper()
	c, err := browse.New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	nav, _, err := c.Build()
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	idx, err := search.BuildIndexWithReader(nav, func(src string) (string, error) { return renderer.ToPlainText([]byte(src)) }, c.ReadFile)
	if err != nil {
		t.Fatal(err)
	}
	basePath := browse.BasePath
	cfg := &config.SiteConfig{Title: "Browse", BasePath: &basePath}
	cfg.Defaults()
	repo, _ := filepath.Abs("../..")
	srv, err := New(Config{SiteCfg: cfg, Browse: c, BrowseWatch: true, EditMode: true, DevMode: true}, os.DirFS(repo), nav, idx, renderer)
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func browseRequest(t *testing.T, s *Server, target string, status int) string {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
	if rec.Code != status {
		t.Fatalf("%s: got %d, want %d: %s", target, rec.Code, status, rec.Body.String())
	}
	return rec.Body.String()
}

func TestBrowseSingleFileOnlyServesSelectedDocumentAndReferencedImages(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "České #100%?.md")
	writeTestFile(t, doc, "# Selected\n\n![Image](pic.png)\n\n[Sibling](other.md)\n\n[Private](secret.json)\n")
	writeTestFile(t, filepath.Join(dir, "other.md"), "# PRIVATE SIBLING")
	writeTestFile(t, filepath.Join(dir, "secret.json"), "PRIVATE JSON")
	writeTestFile(t, filepath.Join(dir, "pic.png"), "IMAGE CONTENT")
	srv := newBrowseTestServer(t, doc)
	pageURL := srv.getNav().Children[0].FullPath
	index := browseRequest(t, srv, "/browse/", 200)
	if !strings.Contains(index, `id="edit-page-btn"`) || !strings.Contains(index, `id="frontmatter-edit-btn"`) {
		t.Fatal("directory overview lacks HTMX edit toolbar targets")
	}
	body := browseRequest(t, srv, pageURL, 200)
	browseRequest(t, srv, srv.editPageURLFor(pageURL), 200)
	if strings.Contains(body, "PRIVATE") || !strings.Contains(body, `/browse/_blocked`) || !strings.Contains(body, `data-browse-status`) {
		t.Fatal("wrong reader output")
	}
	assetURL := regexp.MustCompile(`src="(/browse/_asset/[a-f0-9]+)"`).FindStringSubmatch(body)
	if len(assetURL) != 2 {
		t.Fatal("image was not mapped to a restricted route")
	}
	if got := browseRequest(t, srv, assetURL[1], 200); got != "IMAGE CONTENT" {
		t.Fatal(got)
	}
	for _, path := range []string{"/browse/other.md", "/browse/secret.json", "/browse/_download/secret.json", "/browse/_asset/not-registered", "/browse/_git/history", "/browse/_edit/other.md"} {
		if got := browseRequest(t, srv, path, 404); strings.Contains(got, "PRIVATE") {
			t.Fatalf("leaked %s", path)
		}
	}
	search := browseRequest(t, srv, "/browse/_search?q=PRIVATE", 200)
	if strings.Contains(search, "PRIVATE SIBLING") {
		t.Fatal("search indexed sibling")
	}
	if err := os.Remove(doc); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "other.md"), doc); err != nil {
		t.Skip(err)
	}
	if got := browseRequest(t, srv, pageURL, 500); strings.Contains(got, "PRIVATE SIBLING") {
		t.Fatal("symlink replacement leaked")
	}
}

func TestBrowseCrossRootLinksAndMarkdownEditor(t *testing.T) {
	base := t.TempDir()
	dirA, dirB := filepath.Join(base, "a"), filepath.Join(base, "b")
	docA, docB := filepath.Join(dirA, "README.md"), filepath.Join(dirB, "Spec.markdown")
	writeTestFile(t, docA, "# Alpha\n\n[Spec](../b/Spec.markdown?q=one#section)\n")
	writeTestFile(t, docB, "# Spec\n\n## Section\n")
	srv := newBrowseTestServer(t, dirA, dirB)
	nav := srv.getNav()
	pageA, pageB := nav.Children[0].Children[0].FullPath, nav.Children[1].Children[0].FullPath
	body := browseRequest(t, srv, pageA, 200)
	if !strings.Contains(body, pageB+`?q=one#section`) {
		t.Fatal("cross-root link was not rewritten")
	}
	browseRequest(t, srv, srv.editPageURLFor(pageB), 200)
	current, _ := os.ReadFile(docB)
	response := saveEditedDocument(t, srv, url.Values{"page_path": {pageB}, "revision": {contentRevision(current)}, "source": {"# Updated\n"}}, http.StatusOK)
	if !response.OK {
		t.Fatal(response)
	}
	got, _ := os.ReadFile(docB)
	if string(got) != "# Updated\n" {
		t.Fatal("editor did not save selected .markdown file")
	}
	before := browseRequest(t, srv, "/browse/_status", 200)
	srv.Reload(nav, srv.getIdx())
	after := browseRequest(t, srv, "/browse/_status", 200)
	if before == after {
		t.Fatal("watch status did not advance")
	}
}
