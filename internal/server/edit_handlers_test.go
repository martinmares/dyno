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
)

func TestEditPageSupportsRootIndexWithoutExposingFilePath(t *testing.T) {
	srv := newTestServer(t)
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
