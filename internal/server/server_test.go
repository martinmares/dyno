package server

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"mime/multipart"
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

func TestMetadataFilterPrunesNavigationAndSearch(t *testing.T) {
	srv := newTestServer(t)
	srv.siteCfg.Frontmatter = config.FrontmatterConfig{Fields: map[string]config.FrontmatterFieldConfig{
		"owner": {Label: "Owner", Type: "text", Filterable: true},
	}}
	writeTestFile(t, filepath.Join(srv.contentDir, "guides", "api-widget.md"), "---\nowner: alice\n---\n# API Widget\n\nVisible needle.\n")
	writeTestFile(t, filepath.Join(srv.contentDir, "guides", "internal.md"), "---\nowner: bob\n---\n# Internal\n\nHidden needle.\n")
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

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/guides/api-widget?meta.owner=alice", nil)
	srv.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `name="meta.owner"`) || !strings.Contains(body, `value="alice" checked`) {
		t.Fatalf("expected selected owner facet, got: %s", body)
	}
	if strings.Contains(body, `/guides/internal`) {
		t.Fatalf("filtered navigation must hide Bob's page, got: %s", body)
	}
	if !strings.Contains(body, `/guides/api-widget?meta.owner=alice`) {
		t.Fatalf("navigation must preserve active filters, got: %s", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/_search?q=needle&meta.owner=alice", nil)
	req.Header.Set("HX-Request", "true")
	srv.Handler().ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "Hidden") || !strings.Contains(rec.Body.String(), "Visible <mark>needle</mark>") {
		t.Fatalf("search must use the metadata filter, got: %s", rec.Body.String())
	}
}

func TestMetadataFilterSurvivesHTMXGraphAndBacklinkNavigation(t *testing.T) {
	srv := newTestServer(t)
	srv.siteCfg.Frontmatter = config.FrontmatterConfig{Fields: map[string]config.FrontmatterFieldConfig{
		"document-owner": {Label: "Document owner", Type: "text", Filterable: true},
	}}
	writeTestFile(t, filepath.Join(srv.contentDir, "guides", "source.md"), "---\ndocument-owner: martin.mares@datalite.cz\n---\n# Source\n\n[Target](./target.md)\n")
	writeTestFile(t, filepath.Join(srv.contentDir, "guides", "target.md"), "---\ndocument-owner: martin.mares@datalite.cz\n---\n# Target\n")
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

	filterQuery := "?meta.document-owner=martin.mares%40datalite.cz"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/guides/source"+filterQuery, nil)
	req.Header.Set("HX-Request", "true")
	srv.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{
		`id="ego-graph-btn" hx-swap-oob="outerHTML"`,
		`href="/_graph/guides/source` + filterQuery + `"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("filtered HTMX page must update graph URL with %q, got: %s", want, body)
		}
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/_graph/guides/target"+filterQuery, nil)
	req.Header.Set("HX-Request", "true")
	srv.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get("HX-Push-Url"); got != "/_graph/guides/target"+filterQuery {
		t.Fatalf("graph lost filter in HX-Push-Url: %q", got)
	}
	body = rec.Body.String()
	if !strings.Contains(body, `href="/guides/source`+filterQuery+`"`) {
		t.Fatalf("graph backlink lost metadata filter, got: %s", body)
	}
}

func TestEditLinkPreservesMetadataQuery(t *testing.T) {
	srv := newTestServer(t)
	srv.editMode = true

	req := httptest.NewRequest(http.MethodGet, "/?meta.document-owner=martin.mares%40datalite.cz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="/_edit/?meta.document-owner=martin.mares%40datalite.cz"`) {
		t.Fatalf("expected edit link to preserve query, got: %s", body)
	}
	if !strings.Contains(body, `aria-label="Edit frontmatter"`) {
		t.Fatalf("expected frontmatter edit button in edit mode, got: %s", body)
	}
}

func TestPageCommentsCanBeAddedAndRendered(t *testing.T) {
	srv := newTestServer(t)
	srv.siteCfg.Comments.DocumentIDField = "comment_id"
	writeTestFile(t, filepath.Join(srv.contentDir, "index.md"), "---\ncomment_id: home\n---\n# Home\n\nWelcome.\n")
	store, err := comments.NewJSONLStore(filepath.Join(t.TempDir(), "comments.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	srv.comments = store
	srv.commentsManagement = "all"
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
	var first struct {
		comments.Comment
		BodyHTML    string `json:"body_html"`
		CommentHTML string `json:"comment_html"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.DocumentID != "home" {
		t.Fatalf("comment used wrong document identity: %#v", first)
	}
	for _, want := range []string{"Reply", "Edit", "Delete"} {
		if !strings.Contains(first.CommentHTML, want) {
			t.Fatalf("expected %q in comment html, got: %s", want, first.CommentHTML)
		}
	}

	highlightForm := strings.NewReader("page_path=/&kind=highlight&quote=Welcome")
	req = httptest.NewRequest(http.MethodPost, "/_comments", highlightForm)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected highlight 200, got %d: %s", rec.Code, rec.Body.String())
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

	updateForm := strings.NewReader("body=Looks+*updated*&author=Alicia")
	req = httptest.NewRequest(http.MethodPatch, "/_comments/"+first.ID+"?page_path=/", updateForm)
	req.SetPathValue("id", first.ID)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.updateCommentHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var updated struct {
		comments.Comment
		BodyHTML    string `json:"body_html"`
		CommentHTML string `json:"comment_html"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Author != "Alicia" {
		t.Fatalf("expected updated author, got %#v", updated.Comment)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected page 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Comments", "Highlights", "Alicia", "<em>updated</em>", "Selected text", "Bob", "Re: agreed", "RE", `data-comment-highlight`, `data-quote="Welcome"`, `data-annotation-edit`, `data-annotation-delete`, `data-delete-dialog`, "Delete annotation?"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in rendered page, got: %s", want, body)
		}
	}
	if strings.Contains(body, `data-annotation-kind="highlight"`) {
		t.Fatalf("highlights must not expose an edit action, got: %s", body)
	}
}

func TestBulkFrontmatterSaveAcceptsMultipartForm(t *testing.T) {
	srv := newTestServer(t)
	srv.editMode = true
	srv.siteCfg.Frontmatter = config.FrontmatterConfig{
		Fields: map[string]config.FrontmatterFieldConfig{
			"document-owner": {Label: "Document owner", Type: "text", Filterable: true},
		},
		Defaults: map[string]any{
			"document-owner": "anonymous",
		},
	}
	original := `---
title: "API Widget"
# Keep this comment.
document-owner: alice
---
# API Widget
`
	writeTestFile(t, filepath.Join(srv.contentDir, "guides", "api-widget.md"), original)
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

	current, err := os.ReadFile(filepath.Join(srv.contentDir, "guides", "api-widget.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows := []bulkFrontmatterRowPayload{{
		PagePath: "/guides/api-widget",
		Revision: contentRevision(current),
		Values: map[string]string{
			"document-owner": "bob",
		},
	}}
	payload, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("rows", string(payload)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/_edit/frontmatter/bulk/save", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.bulkFrontmatterSaveHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp bulkFrontmatterSaveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || len(resp.Rows) != 1 || !resp.Rows[0].OK {
		t.Fatalf("unexpected response: %+v", resp)
	}
	updated, err := os.ReadFile(filepath.Join(srv.contentDir, "guides", "api-widget.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	for _, want := range []string{
		`title: "API Widget"`,
		`# Keep this comment.`,
		`document-owner: bob`,
		`# API Widget`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in updated source, got: %s", want, text)
		}
	}
	if strings.Contains(text, `document-owner: alice`) {
		t.Fatalf("expected only targeted field update, got: %s", text)
	}
}

func TestCommentsDisablePageConditionalNotModified(t *testing.T) {
	srv := newTestServer(t)
	srv.siteCfg.Comments.DocumentIDField = "comment_id"
	writeTestFile(t, filepath.Join(srv.contentDir, "index.md"), "---\ncomment_id: home\n---\n# Home\n")
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

func TestSidebarSectionsHaveIndependentToggleAndTitleTooltips(t *testing.T) {
	srv := newTestServer(t)
	writeTestFile(t, filepath.Join(srv.contentDir, "guides", "index.md"), "# Guides\n")
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

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`data-nav-section data-nav-path="/guides/"`,
		`data-nav-toggle aria-expanded="true"`,
		`data-nav-expand-all`,
		`Expand all`,
		`aria-controls="nav-children-%2Fguides%2F"`,
		`title="Guides"`,
		`title="Api Widget"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected sidebar markup %q, got: %s", want, body)
		}
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
	writeTestFile(t, filepath.Join(contentA, "guides", "first.md"), "---\nowner: alice\n---\n# First\n\nLibrary search target alpha.\n")
	writeTestFile(t, filepath.Join(contentB, "index.md"), "# Beta\n\nOther docs.\n")
	writeTestFile(t, filepath.Join(contentB, "guides", "second.md"), "---\nowner: bob\n---\n# Second\n\nAnother alpha match.\n")

	cfgA := "title: Alpha Docs\nslug: alpha\nbase_path: /docs\nfrontmatter:\n  fields:\n    owner:\n      filterable: true\n"
	cfgB := "title: Beta Docs\nslug: beta\nfrontmatter:\n  fields:\n    owner:\n      filterable: true\n"
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
	if !strings.Contains(rec.Body.String(), `hx-get="/docs/_search"`) {
		t.Fatalf("expected dashboard search to use base path, got: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/docs/?meta.owner=alice", nil)
	ls.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Alpha Docs") || strings.Contains(rec.Body.String(), "Beta Docs") {
		t.Fatalf("library dashboard must filter books by metadata, got: %s", rec.Body.String())
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

func TestLibraryRejectsDuplicateBookSlugs(t *testing.T) {
	books := []*library.Book{
		{Slug: "docs", ContentDir: "/repos/security/docs"},
		{Slug: "docs", ContentDir: "/repos/admin/docs"},
	}

	_, err := NewLibrary(LibraryConfig{}, nil, books, nil)
	if err == nil {
		t.Fatal("expected duplicate library slug to fail")
	}
	for _, expected := range []string{"duplicate library slug \"docs\"", "/repos/security/docs", "/repos/admin/docs", "dyno.yaml"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected error to contain %q, got: %v", expected, err)
		}
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
