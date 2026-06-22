package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFrontmatterConfiguration(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`title: Metadata docs
frontmatter:
  display: [document-status, document-owner]
  fields:
    document-owner:
      label: Owner
      type: text
      filterable: true
    updated:
      type: datetime
      readonly: true
      update_on_save: true
    document-status:
      label: Status
      type: select
      options: [NEW, DRAFT, FINAL]
`)
	if err := os.WriteFile(filepath.Join(dir, "dyno.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	fields := cfg.Frontmatter.OrderedFields()
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
	if fields[0].Name != "document-status" || fields[1].Name != "document-owner" || fields[2].Name != "updated" {
		t.Fatalf("unexpected field order: %#v", fields)
	}
	if fields[0].Type != "select" || len(fields[0].Options) != 3 {
		t.Fatalf("unexpected select field: %#v", fields[0])
	}
	if !fields[2].ReadOnly {
		t.Fatalf("expected updated to be read-only")
	}
	if !fields[2].UpdateOnSave {
		t.Fatal("expected updated to be refreshed on save")
	}
	if !cfg.Frontmatter.Fields["document-owner"].Filterable {
		t.Fatal("expected owner to be filterable")
	}
}

func TestLoadCommentsDocumentIDField(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dyno.yaml"), []byte("comments:\n  document_id_field: external_document_id\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Comments.DocumentIDField != "external_document_id" {
		t.Fatalf("unexpected comment identity field %q", cfg.Comments.DocumentIDField)
	}
}

func TestLoadRejectsUpdateOnSaveForTextField(t *testing.T) {
	dir := t.TempDir()
	data := []byte("frontmatter:\n  fields:\n    owner:\n      type: text\n      update_on_save: true\n")
	if err := os.WriteFile(filepath.Join(dir, "dyno.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("expected update_on_save on text field to fail")
	}
}

func TestLoadRejectsInvalidFrontmatterField(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`frontmatter:
  fields:
    status:
      type: magic
`)
	if err := os.WriteFile(filepath.Join(dir, "dyno.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("expected invalid frontmatter field type to fail")
	}
}

func TestMergeDefaultsCopiesFrontmatterConfiguration(t *testing.T) {
	local := SiteConfig{}
	external := SiteConfig{Frontmatter: FrontmatterConfig{
		Fields: map[string]FrontmatterFieldConfig{
			"status": {Type: "select", Options: []string{"DRAFT", "FINAL"}},
		},
	}}
	local.MergeDefaults(external)
	if got := local.Frontmatter.Fields["status"].Type; got != "select" {
		t.Fatalf("expected library frontmatter fallback, got %q", got)
	}

	local.Frontmatter = FrontmatterConfig{Fields: map[string]FrontmatterFieldConfig{"owner": {Type: "text"}}}
	local.MergeDefaults(external)
	if _, exists := local.Frontmatter.Fields["status"]; exists {
		t.Fatal("site frontmatter configuration must win over library fallback")
	}
}
