package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/browse"
	"github.com/mares/dyno/internal/config"
)

func TestPinIdentityIsIndependentOfURLVersionAndSymlinks(t *testing.T) {
	root := t.TempDir()
	s := &Server{contentDir: root, siteCfg: &config.SiteConfig{Title: "Docs"}}
	first := s.pinSource()
	s.basePath, s.siteCfg.Version, s.siteCfg.Title = "/different", "2.0", "Renamed"
	if got := s.pinSource(); got.ID != first.ID || got.Base != "/different" {
		t.Fatalf("mount/version/title must not reset pins: %+v", got)
	}
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	s.contentDir = link
	if s.pinSource().ID != first.ID {
		t.Fatal("symlink must resolve to the same documentation")
	}
	s.contentDir = t.TempDir()
	if s.pinSource().ID == first.ID {
		t.Fatal("different roots on the same origin must not share pins")
	}
	s.siteCfg.StorageID = "shared-docs"
	shared := s.pinSource().ID
	s.contentDir = root
	if s.pinSource().ID != shared {
		t.Fatal("explicit storage_id must survive checkout moves")
	}
	s.siteCfg.StorageID = "other-docs"
	if s.pinSource().ID == shared {
		t.Fatal("explicit identities must be isolated")
	}
	encoded := s.pinSources()
	if strings.Contains(encoded, root) || strings.Contains(encoded, "other-docs") {
		t.Fatalf("filesystem and raw storage IDs must not be exposed: %s", encoded)
	}
	var sources []pinSource
	if err := json.Unmarshal([]byte(encoded), &sources); err != nil || len(sources) != 1 {
		t.Fatalf("invalid pin sources: %s %v", encoded, err)
	}
}

func TestBrowsePinIdentityUsesTheCanonicalSourceSelection(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	makeSource := func(paths ...string) pinSource {
		catalog, err := browse.New(paths, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(catalog.Close)
		return (&Server{browse: catalog, siteCfg: &config.SiteConfig{Title: "Files"}}).pinSource()
	}
	if makeSource(a, b).ID != makeSource(b, a).ID {
		t.Fatal("selection order must not reset pins")
	}
	if makeSource(a).ID == makeSource(b).ID || makeSource(a).ID == makeSource(a, b).ID {
		t.Fatal("different browse selections must be isolated")
	}
}

func TestPinControlsOnlyIdentifyRealDocuments(t *testing.T) {
	s := newTestServer(t, func(cfg *Config) { cfg.Explore = true })
	for _, target := range []string{"/", "/guides/api-widget", "/guides", "/_tasks", "/_search?q=dyno", "/_explore"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", target, nil))
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, "data-pins-open") || !strings.Contains(body, "data-pins-sources=") {
			t.Fatalf("%s: missing pins controls: %d %s", target, w.Code, body)
		}
		wantDocument := target == "/" || target == "/guides/api-widget"
		if strings.Contains(body, "data-pin-document") != wantDocument {
			t.Fatalf("%s: incorrect pinnable document marker", target)
		}
	}
	r := httptest.NewRequest("GET", "/guides/api-widget?q=widget&meta.owner=test", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), `data-pin-path="/guides/api-widget"`) {
		t.Fatalf("HTMX fragment must identify the unfiltered document path: %s", w.Body.String())
	}
}
