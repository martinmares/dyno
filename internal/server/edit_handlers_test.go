package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/frontmatter"
)

func TestEditPageSupportsRootIndexWithoutExposingFilePath(t *testing.T) {
	srv := newTestServer(t)
	srv.siteCfg.Frontmatter = metadataTestConfig()
	req := httptest.NewRequest(http.MethodGet, "/_edit/", nil)
	rec := httptest.NewRecorder()
	srv.editPageHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "let revision = '") {
		t.Fatalf("expected content revision in editor page")
	}
	if strings.Contains(body, srv.contentDir) {
		t.Fatalf("editor page exposed absolute content path")
	}
	for _, want := range []string{"Metadata", "Document metadata", `data-name="document-status"`, `data-name="document-owner"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in metadata-aware editor", want)
		}
	}
	if !strings.Contains(body, `id="git-file-status"`) || !strings.Contains(body, `style="display: none"`) {
		t.Fatal("expected Git status badge to be hidden for a clean non-Git document")
	}
}

func TestEditPagePreservesQueryInBackLink(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/_edit/?meta.document-owner=martin.mares%40datalite.cz", nil)
	rec := httptest.NewRecorder()
	srv.editPageHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="/?meta.document-owner=martin.mares%40datalite.cz"`) {
		t.Fatalf("expected back link to preserve query, got: %s", body)
	}
}

func TestEditSaveUsesRevisionAndAtomicWrite(t *testing.T) {
	srv := newTestServer(t)
	path := filepath.Join(srv.contentDir, "index.md")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}

	response := saveEditedDocument(t, srv, url.Values{
		"page_path": {"/"},
		"revision":  {contentRevision(original)},
		"source":    {"# Home\n\nChanged.\n"},
	}, http.StatusOK)
	if !response.OK || response.Revision == "" {
		t.Fatalf("unexpected save response: %+v", response)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); got != "# Home\n\nChanged.\n" {
		t.Fatalf("unexpected saved content %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("expected mode 0640, got %04o", got)
	}

	// Retrying an already-applied save is idempotent even with the old revision.
	retry := saveEditedDocument(t, srv, url.Values{
		"page_path": {"/"},
		"revision":  {contentRevision(original)},
		"source":    {string(content)},
	}, http.StatusOK)
	if !retry.OK || retry.Revision != response.Revision {
		t.Fatalf("unexpected retry response: %+v", retry)
	}
}

func TestEditSavePreservesExistingLineEndings(t *testing.T) {
	for _, test := range []struct {
		name     string
		original string
		edited   string
		want     string
	}{
		{
			name:     "LF input is restored after multipart CRLF normalization",
			original: "# Home\n\nOriginal.\n",
			edited:   "# Home\r\n\r\nChanged.\r\n",
			want:     "# Home\n\nChanged.\n",
		},
		{
			name:     "CRLF document remains CRLF",
			original: "# Home\r\n\r\nOriginal.\r\n",
			edited:   "# Home\n\nChanged.\n",
			want:     "# Home\r\n\r\nChanged.\r\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := newTestServer(t)
			path := filepath.Join(srv.contentDir, "index.md")
			if err := os.WriteFile(path, []byte(test.original), 0o644); err != nil {
				t.Fatal(err)
			}
			response := saveEditedDocument(t, srv, url.Values{
				"page_path": {"/"},
				"revision":  {contentRevision([]byte(test.original))},
				"source":    {test.edited},
			}, http.StatusOK)
			if !response.OK {
				t.Fatalf("unexpected save response: %+v", response)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != test.want {
				t.Fatalf("unexpected line endings: got %q, want %q", got, test.want)
			}
		})
	}
}

func TestEditSaveUpdatesConfiguredTimestampOnlyAfterContentChange(t *testing.T) {
	srv := newTestServer(t)
	srv.siteCfg.Frontmatter = config.FrontmatterConfig{Fields: map[string]config.FrontmatterFieldConfig{
		"updated": {Type: "datetime", ReadOnly: true, UpdateOnSave: true},
	}}
	path := filepath.Join(srv.contentDir, "index.md")
	original := []byte("---\nupdated: 2025-01-02T03:04:05+01:00\n---\n# Home\n\nOriginal.\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	unchanged := saveEditedDocument(t, srv, url.Values{
		"page_path": {"/"},
		"revision":  {contentRevision(original)},
		"source":    {string(original)},
	}, http.StatusOK)
	if !unchanged.OK {
		t.Fatalf("unexpected unchanged response: %+v", unchanged)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("unchanged save modified timestamp: %q", got)
	}

	started := time.Now().Add(-time.Second)
	response := saveEditedDocument(t, srv, url.Values{
		"page_path": {"/"},
		"revision":  {contentRevision(original)},
		"source":    {"---\nupdated: 2025-01-02T03:04:05+01:00\n---\n# Home\n\nChanged.\n"},
	}, http.StatusOK)
	if !response.OK {
		t.Fatalf("unexpected changed response: %+v", response)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values, err := frontmatter.Read(got)
	if err != nil {
		t.Fatal(err)
	}
	updated, ok := values["updated"].(time.Time)
	if !ok || updated.Before(started) || updated.After(time.Now().Add(time.Second)) {
		t.Fatalf("unexpected updated timestamp %#v in %q", values["updated"], got)
	}
	if !strings.Contains(string(got), "Changed.") {
		t.Fatalf("edited content was not saved: %q", got)
	}
}

func TestEditSaveRejectsConcurrentChange(t *testing.T) {
	srv := newTestServer(t)
	path := filepath.Join(srv.contentDir, "index.md")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Home\n\nChanged elsewhere.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	response := saveEditedDocument(t, srv, url.Values{
		"page_path": {"/"},
		"revision":  {contentRevision(original)},
		"source":    {"# Home\n\nMy edit.\n"},
	}, http.StatusConflict)
	if response.Code != "conflict" || response.Revision == "" {
		t.Fatalf("unexpected conflict response: %+v", response)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); got != "# Home\n\nChanged elsewhere.\n" {
		t.Fatalf("conflicting save overwrote file: %q", got)
	}
}

func TestEditSaveRejectsUnknownPage(t *testing.T) {
	srv := newTestServer(t)
	response := saveEditedDocument(t, srv, url.Values{
		"page_path": {"/../outside"},
		"revision":  {strings.Repeat("0", 64)},
		"source":    {"nope"},
	}, http.StatusNotFound)
	if response.Code != "not_found" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func saveEditedDocument(t *testing.T, srv *Server, values url.Values, wantStatus int) editSaveResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/_edit/save", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.editSaveHandler(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("expected status %d, got %d: %s", wantStatus, rec.Code, rec.Body.String())
	}
	var response editSaveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}
