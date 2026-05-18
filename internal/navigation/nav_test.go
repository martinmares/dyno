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

func TestBuildTreeWithFilter(t *testing.T) {
	root := t.TempDir()
	writeNavTestFile(t, filepath.Join(root, "index.md"), "# Home\n")
	writeNavTestFile(t, filepath.Join(root, "README.md"), "# Readme\n")
	writeNavTestFile(t, filepath.Join(root, "docs", "keep.md"), "# Keep\n")
	writeNavTestFile(t, filepath.Join(root, "docs", "drafts", "hidden.md"), "# Hidden\n")
	writeNavTestFile(t, filepath.Join(root, "other.md"), "# Other\n")

	nav, err := BuildTreeWithFilter(root, "", Filter{
		Include: []string{"docs/**/*.md", "README.md"},
		Exclude: []string{"**/drafts/**"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if FindNode(nav, "/docs/keep") == nil {
		t.Fatal("expected included docs page")
	}
	if FindNode(nav, "/readme") == nil {
		t.Fatal("expected explicitly included root page")
	}
	if FindNode(nav, "/other") != nil {
		t.Fatal("expected non-included page to be hidden")
	}
	if FindNode(nav, "/docs/drafts/hidden") != nil {
		t.Fatal("expected excluded page to be hidden")
	}
}

func writeNavTestFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
