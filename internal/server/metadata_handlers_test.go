package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/frontmatter"
)

func TestBuildMetadataFieldsUsesConfiguredOrderAndTypes(t *testing.T) {
	cfg := metadataTestConfig()
	fields, err := buildMetadataFields(cfg, []byte(`---
document-owner: owner@example.test
document-status: DRAFT
updated: 2026-06-19T10:30:00Z
document-tags: [one, two]
---
# Page
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(fields))
	}
	if fields[0].Name != "document-status" || fields[0].Value != "DRAFT" || len(fields[0].Options) != 3 {
		t.Fatalf("unexpected status field: %#v", fields[0])
	}
	if fields[1].Name != "document-owner" || fields[1].Value != "owner@example.test" {
		t.Fatalf("unexpected owner field: %#v", fields[1])
	}
	if fields[2].Name != "document-tags" || fields[2].Value != "one, two" {
		t.Fatalf("unexpected tags field: %#v", fields[2])
	}
	if fields[3].Name != "updated" || fields[3].Value != "2026-06-19T10:30" || !fields[3].ReadOnly {
		t.Fatalf("unexpected updated field: %#v", fields[3])
	}
}

func TestEditMetadataHandlerPatchesOnlySubmittedFields(t *testing.T) {
	srv := newTestServer(t)
	srv.siteCfg.Frontmatter = metadataTestConfig()
	source := `---
external_refs:
  - id: REQ1
    url: https://example.test/REQ1
document-status: DRAFT
document-owner: owner@example.test
updated: 2026-06-19T10:30:00Z
---
# Page
`
	values, _ := json.Marshal(map[string]any{"document-status": "FINAL"})
	req := httptest.NewRequest(http.MethodPost, "/_edit/metadata", strings.NewReader(url.Values{
		"source": {source},
		"values": {string(values)},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.editMetadataHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response metadataApplyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"document-status: FINAL",
		"document-owner: owner@example.test",
		"updated: 2026-06-19T10:30:00Z",
		"  - id: REQ1\n    url: https://example.test/REQ1",
	} {
		if !strings.Contains(response.Source, want) {
			t.Fatalf("expected %q in:\n%s", want, response.Source)
		}
	}
}

func TestEditMetadataHandlerRejectsReadOnlyAndInvalidSelect(t *testing.T) {
	srv := newTestServer(t)
	srv.siteCfg.Frontmatter = metadataTestConfig()
	for name, value := range map[string]any{
		"updated":         "2026-06-20T12:00",
		"document-status": "UNKNOWN",
	} {
		values, _ := json.Marshal(map[string]any{name: value})
		req := httptest.NewRequest(http.MethodPost, "/_edit/metadata", strings.NewReader(url.Values{
			"source": {"---\ndocument-status: DRAFT\n---\n# Page\n"},
			"values": {string(values)},
		}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		srv.editMetadataHandler(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected %s to be rejected, got %d", name, rec.Code)
		}
	}
}

func TestEditFrontmatterHandlerUpdatesAddsAndRemovesFields(t *testing.T) {
	srv := newTestServer(t)
	path := filepath.Join(srv.contentDir, "index.md")
	original := `---
title: Home
owner: alice
status: DRAFT
tags:
  - one
---
# Home
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	payload, err := json.Marshal([]map[string]string{
		{"key": "title", "value": "Home"},
		{"key": "owner", "value": "bob"},
		{"key": "summary", "value": "Added"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/_edit/frontmatter", strings.NewReader(url.Values{
		"page_path": {"/"},
		"revision":  {contentRevision([]byte(original))},
		"entries":   {string(payload)},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.editFrontmatterHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	for _, want := range []string{"title: Home", "owner: bob", "summary: Added"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in:\n%s", want, text)
		}
	}
	for _, want := range []string{"status:", "tags:"} {
		if strings.Contains(text, want) {
			t.Fatalf("did not expect %q in:\n%s", want, text)
		}
	}
}

func TestEditFrontmatterHandlerAcceptsBrowserForm(t *testing.T) {
	srv := newTestServer(t)
	path := filepath.Join(srv.contentDir, "index.md")
	original := "---\ndocument-status: NEW\nhld-deadline: 2026-10-16\n# Preserve this comment.\ncomponents:\n  - Operations-Infra\n  - TSM-Core\nexternal_refs:\n  - id: REQ0124563\n    label: 'Clooney: REQ0124563'\n    url: https://example.test/REQ0124563\n    type: clooney_requirement\n---\n# Page\n\nUnchanged body.\n"
	writeTestFile(t, path, original)
	entries, err := frontmatter.ReadEntries([]byte(original))
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]frontmatterEntryPayload, 0, len(entries))
	for _, entry := range entries {
		if entry.Key == "document-status" {
			entry.Value = "REVISION"
		}
		payload = append(payload, frontmatterEntryPayload{Key: entry.Key, Value: entry.Value})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"page_path": "/", "revision": contentRevision([]byte(original)), "entries": string(encoded),
	} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/_edit/frontmatter", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.editFrontmatterHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantSource := strings.Replace(original, "document-status: NEW", "document-status: REVISION", 1)
	if string(updated) != wantSource {
		t.Fatalf("status edit changed unrelated source:\n%s", updated)
	}
	before, err := frontmatter.Read([]byte(original))
	if err != nil {
		t.Fatal(err)
	}
	after, err := frontmatter.Read(updated)
	if err != nil {
		t.Fatal(err)
	}
	before["document-status"] = "REVISION"
	if !reflect.DeepEqual(before, after) || !strings.HasSuffix(string(updated), "# Page\n\nUnchanged body.\n") {
		t.Fatalf("unexpected document after status change:\n%s", updated)
	}
	// A second save from the stale dialog must not overwrite newer changes.
	stale := httptest.NewRequest(http.MethodPost, "/_edit/frontmatter", strings.NewReader(url.Values{
		"page_path": {"/"}, "revision": {contentRevision([]byte(original))}, "entries": {string(encoded)},
	}.Encode()))
	stale.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.editFrontmatterHandler(rec, stale)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected conflict, got %d: %s", rec.Code, rec.Body.String())
	}
}

func metadataTestConfig() config.FrontmatterConfig {
	cfg := config.FrontmatterConfig{
		Display: []string{"document-status", "document-owner", "document-tags", "updated"},
		Fields: map[string]config.FrontmatterFieldConfig{
			"document-status": {Label: "Status", Type: "select", Options: []string{"NEW", "DRAFT", "FINAL"}},
			"document-owner":  {Label: "Owner", Type: "text"},
			"document-tags":   {Label: "Tags", Type: "tags"},
			"updated":         {Label: "Updated", Type: "datetime", ReadOnly: true},
		},
	}
	if err := cfg.Normalize(); err != nil {
		panic(err)
	}
	return cfg
}
