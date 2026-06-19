package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/config"
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
