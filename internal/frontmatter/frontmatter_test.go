package frontmatter

import (
	"errors"
	"strings"
	"testing"
)

func TestApplyPreservesUntouchedYAMLBlocks(t *testing.T) {
	source := []byte(`---
title: Example
# Keep this comment.
external_refs:
  - id: REQ1
    label: "Requirement"
    url: https://example.test/REQ1
document-status: DRAFT # workflow
attributes:
- name: id
  type: string
---
# Example
`)
	untouched := `# Keep this comment.
external_refs:
  - id: REQ1
    label: "Requirement"
    url: https://example.test/REQ1`
	attributes := `attributes:
- name: id
  type: string`

	updated, err := Apply(source, map[string]any{
		"document-owner":  "owner@example.test",
		"document-status": "FINAL",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	for _, want := range []string{untouched, attributes, "document-status: FINAL # workflow", "document-owner: owner@example.test"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in:\n%s", want, text)
		}
	}
	metadata, err := Read(updated)
	if err != nil {
		t.Fatal(err)
	}
	if metadata["document-status"] != "FINAL" || metadata["document-owner"] != "owner@example.test" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
}

func TestApplyCreatesFrontmatter(t *testing.T) {
	updated, err := Apply([]byte("# Page\n"), map[string]any{
		"status": "DRAFT",
		"tags":   []any{"one", "two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if !strings.HasPrefix(text, "---\nstatus: DRAFT\ntags:\n    - one\n    - two\n---\n# Page") {
		t.Fatalf("unexpected source:\n%s", text)
	}
}

func TestReadEntriesAndApplyEntries(t *testing.T) {
	source := []byte(`---
title: Example
status: DRAFT
owner: alice@example.test
tags:
  - one
  - two
---
# Page
`)
	entries, err := ReadEntries(source)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(entries); got != 4 {
		t.Fatalf("expected 4 entries, got %d", got)
	}
	if entries[0].Key != "title" || entries[0].Value != "Example" {
		t.Fatalf("unexpected first entry: %#v", entries[0])
	}
	if entries[3].Key != "tags" || entries[3].Value != "- one\n- two" {
		t.Fatalf("unexpected tags entry: %#v", entries[3])
	}

	updated, err := ApplyEntries(source, []Entry{
		{Key: "title", Value: "Renamed"},
		{Key: "owner", Value: "bob@example.test"},
		{Key: "summary", Value: "Added"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	for _, want := range []string{"title: Renamed", "owner: bob@example.test", "summary: Added"} {
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

func TestApplyEntriesCreatesFrontmatter(t *testing.T) {
	updated, err := ApplyEntries([]byte("# Page\n"), []Entry{
		{Key: "title", Value: "Example"},
		{Key: "tags", Value: "- one\n- two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if !strings.HasPrefix(text, "---\ntitle: Example\ntags:\n    - one\n    - two\n---\n# Page") {
		t.Fatalf("unexpected source:\n%s", text)
	}
}

func TestApplyPreservesCRLF(t *testing.T) {
	source := []byte("---\r\nstatus: DRAFT\r\nowner: old\r\n---\r\n# Page\r\n")
	updated, err := Apply(source, map[string]any{"status": "FINAL"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ReplaceAll(string(updated), "\r\n", ""), "\n") {
		t.Fatalf("introduced bare LF into CRLF document: %q", updated)
	}
	if !strings.Contains(string(updated), "status: FINAL\r\nowner: old") {
		t.Fatalf("unexpected source: %q", updated)
	}
}

func TestReadRejectsInvalidFrontmatter(t *testing.T) {
	_, err := Read([]byte("---\nstatus: [broken\n---\n# Page\n"))
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}
