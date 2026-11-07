package server

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/search"
)

func TestSearchPagesAndQuickResults(t *testing.T) {
	s := newTestServer(t)
	idx := search.New()
	for i := range 45 {
		idx.Add(search.Document{Path: fmt.Sprintf("/item-%03d", i), Title: "Database", Body: "Migration instructions.", Sections: []search.Section{{Title: "Database / Migration", Heading: "Migration", Anchor: "migration", Body: "Migration instructions."}}})
	}
	s.Reload(s.getNav(), idx)
	for _, quick := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/_search?q=migration&page=1", nil)
		if quick {
			r.Header.Set("HX-Request", "true")
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, "45 results") {
			t.Fatalf("missing total: %d %s", w.Code, body)
		}
		if quick {
			if !strings.Contains(body, "Open all search results") || !strings.Contains(body, `/item-000?q=migration#migration`) {
				t.Fatalf("quick search must start at zero and link to chapter: %s", body)
			}
		} else {
			if !strings.Contains(body, `/item-020?q=migration#migration`) || !strings.Contains(body, `page=2&amp;q=migration`) || !strings.Contains(body, `page=0&amp;q=migration`) {
				t.Fatalf("missing pagination/chapter links: %s", body)
			}
		}
	}
	r := httptest.NewRequest("GET", "/guides/api-widget?q=syntaxe&_search_anchor=syntaxe", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if got := w.Header().Get("HX-Push-Url"); got != "/guides/api-widget?q=syntaxe#syntaxe" {
		t.Fatalf("reader dropped search heading: %q", got)
	}
}
