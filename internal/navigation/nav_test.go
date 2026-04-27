package navigation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildTreeNormalizesUnicodeSlugs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "01_BSS", "04_Order Management")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(dir, "Digitální kanály objednávky a samoobsluha.md")
	if err := os.WriteFile(page, []byte("# Title\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	nav, err := BuildTree(root, "/docs/wiki")
	if err != nil {
		t.Fatal(err)
	}

	orderMgmt := FindNode(nav, "/docs/wiki/bss/order-management/")
	if orderMgmt == nil {
		t.Fatal("expected normalized directory path")
	}

	pageNode := FindNode(nav, "/docs/wiki/bss/order-management/digitalni-kanaly-objednavky-a-samoobsluha")
	if pageNode == nil {
		t.Fatal("expected normalized page path")
	}
	if pageNode.Title != "Digitální Kanály Objednávky A Samoobsluha" {
		t.Fatalf("unexpected title: %q", pageNode.Title)
	}
}
