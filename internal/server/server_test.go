package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	siteRoot := t.TempDir()
	writeTestFile(t, filepath.Join(siteRoot, "site", "index.md"), "# Home\n\nWelcome.\n")
	writeTestFile(t, filepath.Join(siteRoot, "site", "guides", "api-widget.md"), "# API Widget\n\n## Syntaxe\n\nText about dyno.\n\n```api\nGET https://example.com/get\n```\n")

	cfg := &config.SiteConfig{Title: "Test Docs"}
	basePath := ""
	cfg.BasePath = &basePath
	cfg.Defaults()

	nav, err := navigation.BuildTree(siteRoot, cfg.GetBasePath())
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
		SiteRoot: siteRoot,
		DevMode:  true,
		SiteCfg:  cfg,
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
	_, err := srv.getRenderedPage(filepath.Join(srv.siteRoot, "site", "guides", "api-widget.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = srv.getRenderedPage(filepath.Join(srv.siteRoot, "site", "guides", "api-widget.md"))
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
}
