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

func TestBuildTreeUsesReadmeAsLandingPageWhenIndexIsMissing(t *testing.T) {
	root := t.TempDir()
	rootReadme := filepath.Join(root, "README.md")
	sectionReadme := filepath.Join(root, "docs", "README.md")
	writeNavTestFile(t, rootReadme, "---\ntitle: Repository Home\n---\n# Home\n")
	writeNavTestFile(t, sectionReadme, "---\ntitle: Documentation\n---\n# Docs\n")
	writeNavTestFile(t, filepath.Join(root, "docs", "page.md"), "# Page\n")

	nav, err := BuildTree(root, "/docs")
	if err != nil {
		t.Fatal(err)
	}
	if nav.FSPath != rootReadme || nav.Title != "Repository Home" {
		t.Fatalf("expected root README landing page, got %#v", nav)
	}
	section := FindNode(nav, "/docs/docs/")
	if section == nil || section.FSPath != sectionReadme || section.Title != "Documentation" {
		t.Fatalf("expected section README landing page, got %#v", section)
	}
	if FindNode(nav, "/docs/readme") != nil || FindNode(nav, "/docs/docs/readme") != nil {
		t.Fatal("README landing pages must not also appear as child pages")
	}
}

func TestBuildTreePrefersIndexAndKeepsReadmeAsPage(t *testing.T) {
	root := t.TempDir()
	index := filepath.Join(root, "index.md")
	writeNavTestFile(t, index, "# Home\n")
	writeNavTestFile(t, filepath.Join(root, "README.md"), "# Readme\n")

	nav, err := BuildTree(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if nav.FSPath != index {
		t.Fatalf("expected index landing page, got %q", nav.FSPath)
	}
	if FindNode(nav, "/readme") == nil {
		t.Fatal("README should remain a page when index exists")
	}
}

func TestBuildTreeUsesGroupAsLowestPriorityLandingPage(t *testing.T) {
	root := t.TempDir()
	group := filepath.Join(root, "section", "_group.md")
	writeNavTestFile(t, group, "---\ntitle: Group Landing\n---\n# Group\n")
	writeNavTestFile(t, filepath.Join(root, "section", "child.md"), "# Child\n")

	nav, err := BuildTree(root, "")
	if err != nil {
		t.Fatal(err)
	}
	section := FindNode(nav, "/section/")
	if section == nil || section.FSPath != group || section.Title != "Group Landing" {
		t.Fatalf("expected _group.md landing page, got %#v", section)
	}
	if FindNode(nav, "/section/group") != nil {
		t.Fatal("_group.md must not also appear as a child page")
	}

	readme := filepath.Join(root, "section", "README.md")
	writeNavTestFile(t, readme, "# Readme\n")
	nav, err = BuildTree(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if section = FindNode(nav, "/section/"); section == nil || section.FSPath != readme {
		t.Fatalf("expected README.md to take precedence, got %#v", section)
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
