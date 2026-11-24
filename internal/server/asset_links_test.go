package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/navigation"
)

func TestSiteAssetLinksUseSourceDirectory(t *testing.T) {
	srv := newTestServer(t)
	srv.basePath = "/docs"
	source := filepath.Join(srv.contentDir, "Návrh platformy", "Monitoring.md")
	writeTestFile(t, source, "# Monitoring\n")
	writeTestFile(t, filepath.Join(filepath.Dir(source), "attachments", "schéma & síť.svg"), "<svg/>")
	writeTestFile(t, filepath.Join(srv.contentDir, "shared", "icon.png"), "png")

	tests := []struct{ name, input, want string }{
		{
			"SVG with encoded Czech, spaces, query and fragment",
			`<img src="attachments/sch%C3%A9ma%20%26%20s%C3%AD%C5%A5.svg?v=1&amp;theme=light#overview" alt="Diagram">`,
			`<img src="/docs/N%C3%A1vrh%20platformy/attachments/sch%C3%A9ma%20&amp;%20s%C3%AD%C5%A5.svg?v=1&amp;theme=light#overview" alt="Diagram">`,
		},
		{"parent asset link", `<a href="../shared/icon.png">Open</a>`, `<a href="/docs/shared/icon.png">Open</a>`},
		{"media poster", `<video poster="../shared/icon.png"></video>`, `<video poster="/docs/shared/icon.png"></video>`},
		{"external image", `<img src="https://example.test/icon.png">`, ""},
		{"protocol-relative image", `<img src="//example.test/icon.png">`, ""},
		{"data URL", `<img src="data:image/svg+xml;base64,PHN2Zy8+">`, ""},
		{"root asset", `<img src="/shared/icon.png">`, ""},
		{"mounted asset", `<img src="/docs/shared/icon.png">`, ""},
		{"fragment", `<a href="#overview">Overview</a>`, ""},
		{"missing asset", `<img src="missing.svg">`, ""},
		{"directory", `<a href="attachments/">Folder</a>`, ""},
		{"outside site", `<img src="../../outside.svg">`, ""},
		{"malformed escape", `<img src="bad%xx.svg">`, ""},
		{"Markdown link handled separately", `<a href="Monitoring.md">Monitoring</a>`, ""},
		{"literal code", `<pre><code>&lt;img src="../shared/icon.png"&gt;</code></pre>`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want
			if want == "" {
				want = tc.input
			}
			if got := srv.rewriteSiteAssetLinks(tc.input, source); got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		})
	}
}

func TestSiteSVGImageFromSluggedDocument(t *testing.T) {
	for _, base := range []string{"", "/docs"} {
		t.Run("mount="+base, func(t *testing.T) {
			srv := newTestServer(t, func(cfg *Config) { cfg.SiteCfg.BasePath = &base })
			dir := filepath.Join(srv.contentDir, "Návrh platformy")
			source := filepath.Join(dir, "Monitoring.md")
			markdown := "# Monitoring\n\n![Discovery](<attachments/discovery.svg>)\n"
			writeTestFile(t, source, markdown)
			svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`
			writeTestFile(t, filepath.Join(dir, "attachments", "discovery.svg"), svg)
			nav, err := navigation.BuildTree(srv.contentDir, base)
			if err != nil {
				t.Fatal(err)
			}
			srv.nav = nav
			node := navigation.FindNodeByFSPath(nav, source)
			if node == nil {
				t.Fatal("document not found in navigation")
			}
			imageURL := base + "/N%C3%A1vrh%20platformy/attachments/discovery.svg"
			for _, htmx := range []bool{false, true} {
				req := httptest.NewRequest(http.MethodGet, node.FullPath, nil)
				if htmx {
					req.Header.Set("HX-Request", "true")
				}
				rec := httptest.NewRecorder()
				srv.Handler().ServeHTTP(rec, req)
				if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `src="`+imageURL+`"`) {
					t.Fatalf("image path incorrect (HTMX=%v), status %d", htmx, rec.Code)
				}
			}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, imageURL, nil))
			if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/svg+xml") || rec.Body.String() != svg {
				t.Fatalf("SVG not served: status=%d, type=%s", rec.Code, rec.Header().Get("Content-Type"))
			}
			preview, _, err := srv.renderComparedMarkdown([]byte(markdown), []byte(markdown), source)
			if err != nil || !strings.Contains(string(preview), `src="`+imageURL+`"`) {
				t.Fatalf("comparison preview must use the same asset URL: %s, %v", preview, err)
			}
			changes, _, _, _, err := srv.buildChangeReport(nil, []byte(markdown), source)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, change := range changes {
				found = found || strings.Contains(string(change.After), `src="`+imageURL+`"`)
			}
			if !found {
				t.Fatal("change report must use the same asset URL")
			}
		})
	}
}
