package comments

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONLStoreListAllReplaysUpdatesAndDeletesAcrossDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comments.jsonl")
	store, err := NewJSONLStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if items, err := store.ListAll(); err != nil || len(items) != 0 {
		t.Fatalf("missing file should be empty: %v, %v", items, err)
	}
	first, err := store.Add(Comment{DocumentID: "one", PagePath: "/old", Body: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Add(Comment{DocumentID: "two", PagePath: "/two", Body: "Delete me"})
	if err != nil {
		t.Fatal(err)
	}
	first.Body, first.PagePath = "Updated", "/renamed"
	if _, err := store.Update(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(second.DocumentID, second.PagePath, second.ID); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString("\ninvalid JSON\n" + `{"id":"legacy","page_path":"/legacy","body":"Legacy note"}` + "\n")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("write legacy event: %v, %v", writeErr, closeErr)
	}
	items, err := store.ListAll()
	if err != nil || len(items) != 2 {
		t.Fatalf("expected updated and legacy notes, got %+v, %v", items, err)
	}
	if items[0].ID != "legacy" || items[1].ID != first.ID || items[1].Body != "Updated" || items[1].PagePath != "/renamed" {
		t.Fatalf("incorrect materialized comments: %+v", items)
	}
}

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
