package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

func TestEndToEndSmokeFlow(t *testing.T) {
	srv := newTestServer(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"path":"` + r.URL.Path + `"}`))
	}))
	defer upstream.Close()

	writeTestFile(t, filepath.Join(srv.contentDir, "guides", "api-widget.md"), "# API Widget\n\n## Syntaxe\n\nText about dyno.\n\n```api\nGET "+upstream.URL+"/get\n```\n")
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
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Home") {
		t.Fatalf("homepage failed: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/search?q=dyno", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "/guides/api-widget?q=dyno") {
		t.Fatalf("search failed: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/guides/api-widget?q=dyno", nil)
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("HX-Push-Url"), "?q=dyno") {
		t.Fatalf("htmx page load failed: %d %s", rec.Code, rec.Body.String())
	}

	body, _ := json.Marshal(map[string]any{
		"method":  "GET",
		"url":     upstream.URL + "/get",
		"headers": map[string]string{},
		"body":    "",
	})
	req = httptest.NewRequest(http.MethodPost, "/api-proxy", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "applyProxyResponse") {
		t.Fatalf("api proxy failed: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "dyno_api_proxy_requests_total") {
		t.Fatalf("metrics failed: %d %s", rec.Code, rec.Body.String())
	}
}
