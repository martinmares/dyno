package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/comments"
	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/library"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	siteRoot := t.TempDir()
	contentDir := filepath.Join(siteRoot, "site")
	writeTestFile(t, filepath.Join(contentDir, "index.md"), "# Home\n\nWelcome.\n")
	writeTestFile(t, filepath.Join(contentDir, "guides", "api-widget.md"), "# API Widget\n\n## Syntaxe\n\nText about dyno.\n\n```api\nGET https://example.com/get\n```\n")

	cfg := &config.SiteConfig{Title: "Test Docs"}
	basePath := ""
	cfg.BasePath = &basePath
	cfg.Defaults()

	nav, err := navigation.BuildTree(contentDir, cfg.GetBasePath())
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	idx, err := search.BuildIndex(nav, func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	})
	if err != nil {
		t.Fatal(err)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	staticFS := os.DirFS(repoRoot)
	srv, err := New(Config{
		SiteRoot:   siteRoot,
		ContentDir: contentDir,
		DevMode:    true,
		SiteCfg:    cfg,
	}, staticFS, nav, idx, renderer)
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHealthEndpoints(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.Handler()

	for _, path := range []string{"/healthz", "/livez", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, rec.Code)
		}
		if strings.TrimSpace(rec.Body.String()) != "OK" {
			t.Fatalf("%s returned unexpected body %q", path, rec.Body.String())
		}
	}
}

func TestPageRendersExternalRefs(t *testing.T) {
	srv := newTestServer(t)
	writeTestFile(t, filepath.Join(srv.contentDir, "linked.md"), `---
title: Linked
external_refs:
  - id: PROJ128
    label: Project PROJ128
    url: https://projects.example.test/PROJ128
    type: project
---
# Linked

Body.
`)
	nav, err := navigation.BuildTree(srv.contentDir, srv.basePath)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := search.BuildIndex(nav, func(src string) (string, error) {
		return srv.renderer.ToPlainText([]byte(src))
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.Reload(nav, idx)

	req := httptest.NewRequest(http.MethodGet, "/linked", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Links") {
		t.Fatalf("expected external refs heading, got: %s", body)
	}
	if !strings.Contains(body, `href="https://projects.example.test/PROJ128"`) {
		t.Fatalf("expected external ref URL, got: %s", body)
	}
	if !strings.Contains(body, `target="_blank"`) || !strings.Contains(body, `rel="noopener noreferrer"`) {
		t.Fatalf("expected safe new-window link, got: %s", body)
	}
}

func TestPageCommentsCanBeAddedAndRendered(t *testing.T) {
	srv := newTestServer(t)
	store, err := comments.NewJSONLStore(filepath.Join(t.TempDir(), "comments.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	srv.comments = store
	srv.commentsPath = "/_comments"
	srv.mux.HandleFunc("POST /_comments", srv.addCommentHandler)

	form := strings.NewReader("page_path=/&author=Alice&body=Looks+**good**&anchor=intro&quote=Selected+text")
	req := httptest.NewRequest(http.MethodPost, "/_comments", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var first comments.Comment
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}

	replyForm := strings.NewReader("page_path=/&parent_id=" + first.ID + "&author=Bob&body=Re:+agreed")
	req = httptest.NewRequest(http.MethodPost, "/_comments", replyForm)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected reply 200, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected page 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Comments", "Alice", "<strong>good</strong>", "Selected text", "Bob", "Re: agreed", "RE"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in rendered page, got: %s", want, body)
		}
	}
}

func TestCommentsDisablePageConditionalNotModified(t *testing.T) {
	srv := newTestServer(t)
	store, err := comments.NewJSONLStore(filepath.Join(t.TempDir(), "comments.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	srv.comments = store
	srv.commentsPath = "/_comments"

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", `"anything"`)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with comments enabled, got %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected no-store cache control, got %q", got)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("warmup request returned %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics returned %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "dyno_http_requests_total") {
		t.Fatalf("expected request counter in metrics output, got: %s", body)
	}
	if !strings.Contains(body, "dyno_page_render_cache_total") {
		t.Fatalf("expected page cache metric in metrics output, got: %s", body)
	}
}

func TestHTMXPageFragmentPreservesQueryAndAvoidsNotModified(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/guides/api-widget?q=dyno", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("If-None-Match", `"anything"`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for htmx fragment, got %d", rec.Code)
	}
	if got := rec.Header().Get("HX-Push-Url"); got != "/guides/api-widget?q=dyno" {
		t.Fatalf("unexpected HX-Push-Url %q", got)
	}
	if !strings.Contains(rec.Body.String(), `hx-swap-oob="outerHTML"`) {
		t.Fatalf("expected TOC OOB fragment, got: %s", rec.Body.String())
	}
}

func TestConditionalGetForFullPage(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/guides/api-widget", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected initial 200, got %d", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag on full page response")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/guides/api-widget", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", rec2.Code)
	}
}

func TestPageRenderCacheMetrics(t *testing.T) {
	srv := newTestServer(t)
	_, err := srv.getRenderedPage(filepath.Join(srv.contentDir, "guides", "api-widget.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = srv.getRenderedPage(filepath.Join(srv.contentDir, "guides", "api-widget.md"))
	if err != nil {
		t.Fatal(err)
	}

	metricFamilies, err := srv.metrics.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() != "dyno_page_render_cache_total" {
			continue
		}
		found = true
	}
	if !found {
		t.Fatal("expected page render cache metric family")
	}
}

func TestAssetURLFingerprintingInProd(t *testing.T) {
	srv := newTestServer(t)

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	assetsFS, err := fs.Sub(os.DirFS(repoRoot), "assets")
	if err != nil {
		t.Fatal(err)
	}
	srv.devMode = false
	srv.assetMeta, err = buildAssetManifest(assetsFS)
	if err != nil {
		t.Fatal(err)
	}

	url := srv.assetURL("app.js")
	if !strings.HasPrefix(url, "/assets/app.js?v=") {
		t.Fatalf("unexpected fingerprinted asset url %q", url)
	}
}

func TestFooterUsesBuildMetadata(t *testing.T) {
	srv := newTestServer(t)
	srv.version = "1.2.3"
	srv.commit = "abc1234"

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "© Test Docs · v1.2.3+abc1234") {
		t.Fatalf("expected footer build metadata, got: %s", body)
	}
	if !strings.Contains(body, "/assets/app.css") {
		t.Fatalf("expected app.css stylesheet link, got: %s", body)
	}
}

func TestSyntheticIndexForMissingIndexPage(t *testing.T) {
	siteRoot := t.TempDir()
	contentDir := filepath.Join(siteRoot, "wiki")
	writeTestFile(t, filepath.Join(contentDir, "guides", "getting-started.md"), "# Getting Started\n\nHello.\n")

	cfg := &config.SiteConfig{Title: "Wiki Docs"}
	basePath := ""
	cfg.BasePath = &basePath
	cfg.Defaults()

	nav, err := navigation.BuildTree(contentDir, cfg.GetBasePath())
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	idx, err := search.BuildIndex(nav, func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	})
	if err != nil {
		t.Fatal(err)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	staticFS := os.DirFS(repoRoot)
	srv, err := New(Config{
		SiteRoot:   siteRoot,
		ContentDir: contentDir,
		DevMode:    true,
		SiteCfg:    cfg,
	}, staticFS, nav, idx, renderer)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/guides/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "This section has no <code>index.md</code>, <code>README.md</code>, or <code>_group.md</code>") {
		t.Fatalf("expected synthetic index note, got: %s", body)
	}
	if !strings.Contains(body, "/guides/getting-started") {
		t.Fatalf("expected child page link, got: %s", body)
	}
}

func TestRewriteMDLinksUsesSourceFileAndDirectoryFallback(t *testing.T) {
	srv := newTestServer(t)
	landing := filepath.Join(srv.contentDir, "detail-design", "platformizace", "README.md")
	targetDir := filepath.Join(srv.contentDir, "detail-design", "platformizace", "REQ0122859 - Uživatelské požadavky (BRD)")
	writeTestFile(t, landing, "# Platformizace\n")
	writeTestFile(t, filepath.Join(targetDir, "_group.md"), "# Requirements\n")

	nav, err := navigation.BuildTree(srv.contentDir, srv.basePath)
	if err != nil {
		t.Fatal(err)
	}
	srv.nav = nav

	got := srv.rewriteMDLinks(
		`<a href="REQ0122859%20-%20U%C5%BEivatelsk%C3%A9%20po%C5%BEadavky%20(BRD)/REQ0122859%20-%20U%C5%BEivatelsk%C3%A9%20po%C5%BEadavky%20(BRD).md">BRD</a>`,
		landing,
	)
	want := `href="/detail-design/platformizace/req0122859-uzivatelske-pozadavky-brd/"`
	if !strings.Contains(got, want) {
		t.Fatalf("expected directory landing URL %s, got %s", want, got)
	}
}

func TestLibrarySearchUsesBasePath(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	contentA := filepath.Join(rootA, "wiki-a")
	contentB := filepath.Join(rootB, "wiki-b")

	writeTestFile(t, filepath.Join(contentA, "index.md"), "# Alpha\n\nShared term.\n")
	writeTestFile(t, filepath.Join(contentA, "guides", "first.md"), "# First\n\nLibrary search target alpha.\n")
	writeTestFile(t, filepath.Join(contentB, "index.md"), "# Beta\n\nOther docs.\n")
	writeTestFile(t, filepath.Join(contentB, "guides", "second.md"), "# Second\n\nAnother alpha match.\n")

	cfgA := "title: Alpha Docs\nslug: alpha\nbase_path: /docs\n"
	cfgB := "title: Beta Docs\nslug: beta\n"
	writeTestFile(t, filepath.Join(contentA, "dyno.yaml"), cfgA)
	writeTestFile(t, filepath.Join(contentB, "dyno.yaml"), cfgB)

	renderer, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	plainText := func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	}

	bookA, err := library.Load(contentA, "/docs", plainText, nil)
	if err != nil {
		t.Fatal(err)
	}
	bookB, err := library.Load(contentB, "/docs", plainText, nil)
	if err != nil {
		t.Fatal(err)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	staticFS := os.DirFS(repoRoot)
	ls, err := NewLibrary(LibraryConfig{
		Title:    "Library",
		LogoText: "Dyno",
		BasePath: "/docs",
		DevMode:  true,
	}, staticFS, []*library.Book{bookA, bookB}, renderer)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	ls.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `hx-get="/docs/search"`) {
		t.Fatalf("expected dashboard search to use base path, got: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/docs/_search?q=alpha", nil)
	req.Header.Set("HX-Request", "true")
	ls.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected library search 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/docs/alpha/guides/first?q=alpha") {
		t.Fatalf("expected alpha result in body, got: %s", body)
	}
	if !strings.Contains(body, "/docs/beta/guides/second?q=alpha") {
		t.Fatalf("expected beta result in body, got: %s", body)
	}
	if strings.Contains(body, `hx-target="#page-content"`) {
		t.Fatalf("expected library search results to use plain links, got: %s", body)
	}
}

func TestLibraryBookPageKeepsSiteHomeAndDashboardLinks(t *testing.T) {
	rootA := t.TempDir()
	contentA := filepath.Join(rootA, "wiki-a")

	writeTestFile(t, filepath.Join(contentA, "index.md"), "# Alpha\n\nHome.\n")
	writeTestFile(t, filepath.Join(contentA, "guides", "first.md"), "# First\n\nBody.\n")
	writeTestFile(t, filepath.Join(contentA, "dyno.yaml"), "title: Alpha Docs\nlogo_text: Alpha\nslug: alpha\nbase_path: /docs\n")

	renderer, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	plainText := func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	}
	bookA, err := library.Load(contentA, "/docs", plainText, nil)
	if err != nil {
		t.Fatal(err)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	staticFS := os.DirFS(repoRoot)
	ls, err := NewLibrary(LibraryConfig{
		Title:    "Library",
		LogoText: "Dyno",
		BasePath: "/docs",
		DevMode:  true,
	}, staticFS, []*library.Book{bookA}, renderer)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs/alpha/guides/first", nil)
	ls.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected book page 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="/docs/alpha/"`) {
		t.Fatalf("expected logo to point to book home, got: %s", body)
	}
	if !strings.Contains(body, `href="/docs/"`) || !strings.Contains(body, `Library`) {
		t.Fatalf("expected dashboard link, got: %s", body)
	}
}

func TestAgentsDocumentGetsVisualMarkers(t *testing.T) {
	siteRoot := t.TempDir()
	contentDir := filepath.Join(siteRoot, "wiki")
	writeTestFile(t, filepath.Join(contentDir, "AGENTS.md"), "# Agents\n\nInstructions.\n")

	cfg := &config.SiteConfig{Title: "Wiki Docs"}
	basePath := ""
	cfg.BasePath = &basePath
	cfg.Defaults()

	nav, err := navigation.BuildTree(contentDir, cfg.GetBasePath())
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	idx, err := search.BuildIndex(nav, func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	})
	if err != nil {
		t.Fatal(err)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	staticFS := os.DirFS(repoRoot)
	srv, err := New(Config{
		SiteRoot:   siteRoot,
		ContentDir: contentDir,
		DevMode:    true,
		SiteCfg:    cfg,
	}, staticFS, nav, idx, renderer)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "nav-badge-agent") {
		t.Fatalf("expected nav AGENT badge, got: %s", body)
	}
	if !strings.Contains(body, "Agent instructions") {
		t.Fatalf("expected page AGENT marker, got: %s", body)
	}
}

func TestTasksBlocksAndTaskListsRender(t *testing.T) {
	siteRoot := t.TempDir()
	contentDir := filepath.Join(siteRoot, "wiki")

	writeTestFile(t, filepath.Join(contentDir, "index.md"), "# Home\n\nWelcome.\n")
	writeTestFile(t, filepath.Join(contentDir, "planning", "roadmap.md"), "# Roadmap\n\n- [x] Done task with `code`.\n- [ ] Open task with `SAP BW`, `ABS`, `tSM` and `41021 -> 41026/41027/41177/41221` inline code.\n")
	writeTestFile(t, filepath.Join(contentDir, "planning", "dashboard.md"), "# Dashboard\n\n- [x] Closed dashboard task.\n- [ ] Keep dashboard task open.\n- [ ] Second dashboard task open.\n\n```tasks\nnot done\npath includes planning/dashboard\nsort by description\n```\n")
	writeTestFile(t, filepath.Join(contentDir, "planning", "overview.md"), "# Tasks Overview\n\n```tasks\npath includes planning\nsort by description\n```\n")

	cfg := &config.SiteConfig{Title: "Wiki Docs"}
	basePath := ""
	cfg.BasePath = &basePath
	cfg.Defaults()

	nav, err := navigation.BuildTree(contentDir, cfg.GetBasePath())
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	idx, err := search.BuildIndex(nav, func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	})
	if err != nil {
		t.Fatal(err)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	staticFS := os.DirFS(repoRoot)
	srv, err := New(Config{
		SiteRoot:   siteRoot,
		ContentDir: contentDir,
		DevMode:    true,
		SiteCfg:    cfg,
	}, staticFS, nav, idx, renderer)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/planning/roadmap", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected roadmap 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="task-list-item"`) {
		t.Fatalf("expected GFM task list markup, got: %s", body)
	}
	if strings.Contains(body, `class="tasks-view"`) {
		t.Fatalf("expected roadmap page to render plain checklist only, got: %s", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/planning/dashboard", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, `class="tasks-view"`) {
		t.Fatalf("expected tasks view wrapper, got: %s", body)
	}
	if !strings.Contains(body, `id="tasks-0"`) {
		t.Fatalf("expected task anchor, got: %s", body)
	}
	if !strings.Contains(body, `id="tasks-0-filter"`) {
		t.Fatalf("expected task filter anchor, got: %s", body)
	}
	if !strings.Contains(body, `href="/planning/dashboard#tasks-0-filter"`) {
		t.Fatalf("expected page badge to point at task filter, got: %s", body)
	}
	if !strings.Contains(body, "All tasks") {
		t.Fatalf("expected global tasks link label, got: %s", body)
	}
	if !strings.Contains(body, `class="tasks-row tasks-row-open"`) {
		t.Fatalf("expected table-like task rows, got: %s", body)
	}
	if !strings.Contains(body, `class="tasks-page-badge"`) {
		t.Fatalf("expected page-level task badge, got: %s", body)
	}
	if !strings.Contains(body, `2 open`) || !strings.Contains(body, `0 done`) {
		t.Fatalf("expected filtered open task counts, got: %s", body)
	}
	if !strings.Contains(body, `class="tasks-row-index">1</span>`) || !strings.Contains(body, `class="tasks-row-index">2</span>`) {
		t.Fatalf("expected local task numbering, got: %s", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/_tasks", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected task summary 200, got %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, "Task list summary") {
		t.Fatalf("expected task summary title, got: %s", body)
	}
	if !strings.Contains(body, `class="tasks-summary-chip"`) {
		t.Fatalf("expected summary page chips, got: %s", body)
	}
	if !strings.Contains(body, `href="#tasks-page-`) {
		t.Fatalf("expected summary page anchors, got: %s", body)
	}
	if !strings.Contains(body, `href="/planning/dashboard#tasks-0"`) {
		t.Fatalf("expected anchor link back to dashboard task block, got: %s", body)
	}
}
