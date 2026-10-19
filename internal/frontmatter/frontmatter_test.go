package frontmatter

import (
	"bytes"
	"errors"
	"fmt"
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

func TestApplyEntriesPreservesOriginalFormatting(t *testing.T) {
	original := `---
# Document properties.
refid: REQ0126066
hld-deadline: 2026-10-16
title: 'Original title'
document-status: NEW # workflow
cetin-analytik:
  - Bumba Bohuslav
cetin-vlastnik:
- Bumba Bohuslav
created: 2026-03-04T13:07:32.425+01:00
updated: 2026-05-05T08:42:47.966+02:00

# Keep the original indentation and key order.
components:
  - OCP
  - OKD
  - TSM-Core
external_refs:
  - id: REQ0126066
    label: "Clooney: REQ0126066"
    url: https://example.test/REQ0126066
    type: clooney_requirement
summary: |-
  First line.
  Second line.
attributes: {z: 1, a: 'two'}
# Keep this trailing comment.
---
# Page

Body without a trailing newline.`
	for _, newline := range []string{"\n", "\r\n"} {
		t.Run(fmt.Sprintf("newline_%q", newline), func(t *testing.T) {
			source := []byte(strings.ReplaceAll(original, "\n", newline))
			entries, err := ReadEntries(source)
			if err != nil {
				t.Fatal(err)
			}
			unchanged, err := ApplyEntries(source, entries)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(unchanged, source) {
				t.Fatalf("saving unchanged fields reformatted the document:\n%s", unchanged)
			}
			for i := range entries {
				if entries[i].Key == "document-status" {
					entries[i].Value = "REVISION"
				}
			}
			updated, err := ApplyEntries(source, entries)
			if err != nil {
				t.Fatal(err)
			}
			want := bytes.Replace(source, []byte("document-status: NEW"), []byte("document-status: REVISION"), 1)
			if !bytes.Equal(updated, want) {
				t.Fatalf("status edit must change only its source line:\nwant:\n%s\ngot:\n%s", want, updated)
			}
		})
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

func TestApplyEntriesReplacesOnlyTheChangedBlock(t *testing.T) {
	for _, block := range []string{
		"summary: |-\n  First line.\n  Second line.\n",
		"summary: >\n  First line.\n  Second line.\n  Third line.\n",
		"summary: [\n  one,\n  two\n]\n",
		"summary: \"First line\n  second line\"\n",
	} {
		t.Run(block, func(t *testing.T) {
			original := "---\ntitle: 'Keep title'\n" + block + "\n# Keep owner comment.\nowner: 'alice'\n---\n# Page\n"
			entries, err := ReadEntries([]byte(original))
			if err != nil {
				t.Fatal(err)
			}
			entries[1].Value = "Changed"
			updated, err := ApplyEntries([]byte(original), entries)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(original, block, "summary: Changed\n", 1)
			if string(updated) != want {
				t.Fatalf("block replacement affected its neighbours:\n%s", updated)
			}
			updated, err = ApplyEntries([]byte(original), append(entries[:1:1], entries[2:]...))
			if err != nil {
				t.Fatal(err)
			}
			if string(updated) != strings.Replace(original, block, "", 1) {
				t.Fatalf("block deletion affected its neighbours:\n%s", updated)
			}
		})
	}
}

func TestReadRejectsInvalidFrontmatter(t *testing.T) {
	_, err := Read([]byte("---\nstatus: [broken\n---\n# Page\n"))
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}
