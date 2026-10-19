package navigation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBacklinksParsePublishedMarkdownLinks(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"index.md":              "---\nexample: '[not a link](hidden.md)'\n---\n# Home\n\n[Bare](guides/Target%20Page.md#part)\n\n[Reference][target]\n\n[target]: guides/Target%20Page.md\n\n`[code](hidden.md)`\n\n```md\n[fenced](hidden.md)\n```\n\n[External](https://example.test/hidden.md)\n[Missing](missing.md)\n",
		"guides/Target Page.md": "# Target\n\n[Home](../index.md)\n[Self](Target%20Page.md)\n",
		"hidden.md":             "# Not linked\n",
	}
	for name, content := range files {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	nav, err := BuildTree(root, "/manual")
	if err != nil {
		t.Fatal(err)
	}
	index := BuildBacklinks(nav, "/manual")
	if got := len(index["/manual/guides/target-page"]); got != 1 {
		t.Fatalf("bare/reference links not resolved or deduplicated: %+v", index)
	}
	if got := len(index["/manual/"]); got != 1 {
		t.Fatalf("directory index link not resolved: %+v", index)
	}
	if len(index["/manual/hidden"]) != 0 {
		t.Fatalf("non-links counted: %+v", index)
	}
}
