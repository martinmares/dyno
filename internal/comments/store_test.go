package comments

import (
	"path/filepath"
	"testing"
)

func TestJSONLStoreUsesDocumentIdentityAndAcceptsHighlights(t *testing.T) {
	store, err := NewJSONLStore(filepath.Join(t.TempDir(), "comments.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Add(Comment{DocumentID: "DOC-1", PagePath: "/old-path", Kind: "highlight", Quote: "selected text"})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.List("DOC-1", "/new-path")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Kind != "highlight" || items[0].Body != "" {
		t.Fatalf("unexpected highlight: %#v", items)
	}
	created.Quote = "replacement text"
	if _, err := store.Update(created); err != nil {
		t.Fatal(err)
	}
	items, err = store.List("DOC-1", "/new-path")
	if err != nil || len(items) != 1 || items[0].Quote != "replacement text" || items[0].Event != "update" {
		t.Fatalf("update event was not materialized: %#v, %v", items, err)
	}
	if err := store.Delete("DOC-1", "/old-path", created.ID); err != nil {
		t.Fatal(err)
	}
	if items, err = store.List("DOC-1", "/old-path"); err != nil || len(items) != 0 {
		t.Fatalf("delete tombstone was not materialized: %#v, %v", items, err)
	}
	if other, err := store.List("DOC-2", "/other-path"); err != nil || len(other) != 0 {
		t.Fatalf("comments leaked across document identities: %#v, %v", other, err)
	}
}
