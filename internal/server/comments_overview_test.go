package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mares/dyno/internal/comments"
	"github.com/mares/dyno/internal/navigation"
)

func commentsOverviewServer(t *testing.T, base string) (*Server, *comments.JSONLStore) {
	t.Helper()
	store, err := comments.NewJSONLStore(filepath.Join(t.TempDir(), "notes.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, func(cfg *Config) {
		cfg.Comments = store
		cfg.SiteCfg.BasePath = &base
		cfg.SiteCfg.Comments.DocumentIDField = "comment_id"
	})
	writeTestFile(t, filepath.Join(srv.contentDir, "guides", "api-widget.md"), "---\ncomment_id: api\n---\n# API Widget\n\n## Metrics\n\nMetrics text.\n")
	writeTestFile(t, filepath.Join(srv.contentDir, "index.md"), "---\ncomment_id: home\n---\n# Home\n")
	nav, err := navigation.BuildTree(srv.contentDir, base)
	if err != nil {
		t.Fatal(err)
	}
	srv.nav = nav
	return srv, store
}

func TestCommentsOverviewGroupsByCurrentDocumentIdentityAndFindsCzech(t *testing.T) {
	srv, _ := commentsOverviewServer(t, "/docs")
	now := time.Now().UTC()
	items := []comments.Comment{
		{ID: "one", DocumentID: "api", PagePath: "/old-name", Author: "Alice", Body: "Prověřit **rozdělení** metrik", Anchor: "metrics", Quote: "Cílový stav", CreatedAt: now},
		{ID: "two", DocumentID: "home", PagePath: "/docs/", Body: "Page note", CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(time.Hour)},
		{ID: "reply", DocumentID: "api", PagePath: "/another-old-name", ParentID: "one", Body: "Reply", CreatedAt: now},
		{ID: "legacy", PagePath: "/docs/guides/api-widget", Body: "Old note", CreatedAt: now},
		{ID: "missing", DocumentID: "removed", PagePath: "javascript:alert(1)", Body: "Missing note", CreatedAt: now},
		{ID: "wrong-identity", DocumentID: "replaced", PagePath: "/docs/", Body: "Replaced document", CreatedAt: now},
		{ID: "highlight", DocumentID: "api", Kind: "highlight", Quote: "Highlight only", CreatedAt: now},
	}
	view := srv.buildCommentsOverview(items, "")
	if view.Total != 6 || view.Shown != 6 || len(view.Groups) != 4 || view.Groups[0].Title != "Home" {
		t.Fatalf("unexpected grouping: %+v", view)
	}
	var api *commentDocumentGroup
	for _, group := range view.Groups {
		if group.URL == "/docs/guides/api-widget" {
			api = group
		}
		if group.Path == "javascript:alert(1)" && group.URL != "" {
			t.Fatal("missing documents must never link to stored paths")
		}
		if group.Title == "/docs/" && group.URL != "" {
			t.Fatal("must not attach a note to a different document reusing an old URL")
		}
	}
	if api == nil || len(api.Entries) != 3 || api.Entries[0].URL != "/docs/guides/api-widget#comment-one" || api.Entries[0].ContextURL != "/docs/guides/api-widget#metrics" || !strings.Contains(string(api.Entries[0].BodyHTML), "<strong>rozdělení</strong>") {
		t.Fatalf("missing current document links or comments: %+v", api)
	}
	for _, query := range []string{"ROZDELENI", "api metrik", "cilovy", "ALICE"} {
		filtered := srv.buildCommentsOverview(items, query)
		if filtered.Shown != 1 || len(filtered.Groups) != 1 || filtered.Groups[0].Entries[0].Comment.ID != "one" {
			t.Fatalf("incorrect search for %q: %+v", query, filtered)
		}
	}
}

func TestCommentsOverviewRouteMenuAndSearch(t *testing.T) {
	for _, base := range []string{"", "/docs"} {
		t.Run(base, func(t *testing.T) {
			srv, store := commentsOverviewServer(t, base)
			comment, err := store.Add(comments.Comment{DocumentID: "api", PagePath: "/old", Author: "Alice", Body: "Prověřit rozdělení <script>alert(1)</script>", Anchor: "metrics", Quote: "Selected text"})
			if err != nil {
				t.Fatal(err)
			}
			for _, route := range []string{base + "/", base + "/_search?q=dyno", base + "/_comments"} {
				rec := httptest.NewRecorder()
				srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", route, nil))
				if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `href="`+base+`/_comments"`) || !strings.Contains(rec.Body.String(), `aria-label="Local comments"`) {
					t.Fatalf("missing menu entry on %s, status %d", route, rec.Code)
				}
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", base+"/_comments?q=ROZDELENI", nil)
			req.Header.Set("HX-Request", "true")
			req.Header.Set("HX-Target", "comments-overview-results")
			srv.Handler().ServeHTTP(rec, req)
			body := rec.Body.String()
			if rec.Code != http.StatusOK || !strings.Contains(body, base+"/guides/api-widget#comment-"+comment.ID) || !strings.Contains(body, "1 of 1 comments") || strings.Contains(body, "<script>alert(1)</script>") || strings.Contains(body, "<html") || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("incorrect overview response: %s", body)
			}
			if err := store.Delete("api", "/old", comment.ID); err != nil {
				t.Fatal(err)
			}
			rec = httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", base+"/_comments", nil))
			if !strings.Contains(rec.Body.String(), "No local comments yet") || strings.Contains(rec.Body.String(), "Prověřit") {
				t.Fatal("deleted comments must disappear from overview")
			}
		})
	}
}

func TestCommentsOverviewDisabled(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if strings.Contains(rec.Body.String(), `aria-label="Local comments"`) {
		t.Fatal("disabled comments must not expose an overview button")
	}
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/_comments", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled overview must return 404, got %d", rec.Code)
	}
}
