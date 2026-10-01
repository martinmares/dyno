package browse

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/navigation"
)

func put(t *testing.T, name, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}

func tempDir(t *testing.T) string {
	t.Helper()
	name, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func TestFilesystemNavigationAndOverlappingSources(t *testing.T) {
	base := tempDir(t)
	a, b := filepath.Join(base, "project-a", "docs"), filepath.Join(base, "project-b", "docs")
	for _, name := range []string{"10-start.MD", "2-start.md", "README.md", "_model/Žluťoučký.md", "nested/spec.markdown", "node_modules/hidden.md", ".hidden/hidden.md", "archive/old.md"} {
		put(t, filepath.Join(a, name), "---\ndraft: true\n---\n# Content")
	}
	put(t, filepath.Join(a, ".pages"), "nav: []")
	put(t, filepath.Join(b, "other.md"), "# Other")
	c, err := New([]string{filepath.Join(a, "README.md"), a, a, b}, []string{"archive/**"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	nav, _, err := c.Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(nav.Children) != 2 || nav.Children[0].Title != "project-a/docs" || nav.Children[1].Title != "project-b/docs" {
		t.Fatalf("unexpected roots: %+v", nav.Children)
	}
	pages := navigation.FlatPages(nav)
	if len(pages) != 6 {
		t.Fatalf("expected 6 selected pages, got %d", len(pages))
	}
	first := nav.Children[0]
	if !first.Children[0].IsDir || first.Children[2].Title != "2-start.md" || first.Children[3].Title != "10-start.MD" {
		t.Fatalf("wrong natural filesystem ordering: %+v", first.Children)
	}
	for _, page := range pages {
		if !strings.Contains(page.FullPath, url.PathEscape(filepath.Base(page.FSPath))) {
			t.Fatalf("filename not preserved: %s", page.FullPath)
		}
	}
}

func TestFileSelectionDoesNotExpandAndRejectsSymlinkReplacement(t *testing.T) {
	base := tempDir(t)
	document, sibling := filepath.Join(base, "selected.md"), filepath.Join(base, "sibling.md")
	put(t, document, "# Selected")
	put(t, sibling, "# PRIVATE")
	put(t, filepath.Join(base, "picture.png"), "image")
	put(t, filepath.Join(base, "data.json"), "secret")
	c, err := New([]string{document}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.ReadFile(sibling); err == nil {
		t.Fatal("sibling document was allowed")
	}
	if _, err := c.OpenAsset(document, sibling); err == nil {
		t.Fatal("Markdown was allowed as an asset")
	}
	if _, err := c.OpenAsset(document, filepath.Join(base, "data.json")); err == nil {
		t.Fatal("non-image attachment was allowed for a standalone file")
	}
	f, err := c.OpenAsset(document, filepath.Join(base, "picture.png"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	outside := filepath.Join(tempDir(t), "private.md")
	put(t, outside, "PRIVATE OUTSIDE")
	if err := os.Remove(document); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, document); err != nil {
		t.Skip(err)
	}
	if _, err := c.ReadFile(document); err == nil {
		t.Fatal("symlink replacement escaped selection")
	}
	if err := c.WriteFile(document, []byte("changed"), 0644); err == nil {
		t.Fatal("symlink replacement was writable")
	}
}

func TestExplicitFileRemainsSelectedWhenParentScanExcludesIt(t *testing.T) {
	base := tempDir(t)
	document := filepath.Join(base, ".hidden", "selected.md")
	put(t, document, "# Explicit")
	put(t, filepath.Join(base, "visible.md"), "# Visible")
	for _, inputs := range [][]string{{base, document}, {document, base}} {
		c, err := New(inputs, nil)
		if err != nil {
			t.Fatal(err)
		}
		nav, _, err := c.Build()
		if err != nil {
			c.Close()
			t.Fatal(err)
		}
		_, readErr := c.ReadFile(document)
		c.Close()
		if len(navigation.FlatPages(nav)) != 2 || readErr != nil {
			t.Fatal("explicit file disappeared during parent deduplication")
		}
	}
}

func TestDirectoryTraversalAndSymlinksStayWithinSource(t *testing.T) {
	base := tempDir(t)
	root := filepath.Join(base, "docs")
	doc := filepath.Join(root, "README.md")
	outside := filepath.Join(base, "private.md")
	put(t, doc, "# Doc")
	put(t, outside, "private")
	put(t, filepath.Join(root, "assets", "pic.png"), "image")
	if err := os.Symlink(base, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	c, err := New([]string{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	nav, first, err := c.Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(navigation.FlatPages(nav)) != 1 {
		t.Fatal("followed descendant symlink")
	}
	for _, target := range []string{outside, filepath.Join(root, "escape", "private.md"), filepath.Join(root, "..", "private.md")} {
		if _, err := c.ReadFile(target); err == nil {
			t.Fatalf("allowed %s", target)
		}
		if _, err := c.OpenAsset(doc, target); err == nil {
			t.Fatalf("allowed asset %s", target)
		}
	}
	put(t, filepath.Join(root, "nested", "new.md"), "# Added")
	nav, second, err := c.Build()
	if err != nil {
		t.Fatal(err)
	}
	if second == first || len(navigation.FlatPages(nav)) != 2 {
		t.Fatal("new nested source did not change snapshot")
	}
}
