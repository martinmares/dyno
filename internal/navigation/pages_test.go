package navigation

import (
	"path/filepath"
	"testing"
)

func TestPagesOrdersAndRenamesWithoutHidingUnlistedChildren(t *testing.T) {
	root := t.TempDir()
	writeNavTestFile(t, filepath.Join(root, "alpha.md"), "# Alpha\n")
	writeNavTestFile(t, filepath.Join(root, "beta.md"), "# Beta\n")
	writeNavTestFile(t, filepath.Join(root, "gamma.md"), "# Gamma\n")
	writeNavTestFile(t, filepath.Join(root, ".pages"), "nav:\n  - Gamma page: gamma.md\n  - alpha.md\n")

	nav, err := BuildTree(root, "")
	if err != nil {
		t.Fatal(err)
	}
	assertNavChildren(t, nav, []string{"gamma.md", "alpha.md", "beta.md"})
	if nav.Children[0].Title != "Gamma page" {
		t.Fatalf("expected .pages title, got %q", nav.Children[0].Title)
	}
}

func TestPagesWildcardControlsPlacementOfUnlistedChildren(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha.md", "beta.md", "gamma.md", "omega.md"} {
		writeNavTestFile(t, filepath.Join(root, name), "# Page\n")
	}
	writeNavTestFile(t, filepath.Join(root, ".pages"), "nav:\n  - gamma.md\n  - ...\n  - alpha.md\n")

	nav, err := BuildTree(root, "")
	if err != nil {
		t.Fatal(err)
	}
	assertNavChildren(t, nav, []string{"gamma.md", "beta.md", "omega.md", "alpha.md"})
}

func TestInvalidPagesDoesNotPreventNavigationBuild(t *testing.T) {
	root := t.TempDir()
	writeNavTestFile(t, filepath.Join(root, "alpha.md"), "# Alpha\n")
	writeNavTestFile(t, filepath.Join(root, ".pages"), "nav: not-a-list\n")

	nav, err := BuildTree(root, "")
	if err != nil {
		t.Fatal(err)
	}
	assertNavChildren(t, nav, []string{"alpha.md"})
}

func TestPagesMissingTargetIsIgnored(t *testing.T) {
	root := t.TempDir()
	writeNavTestFile(t, filepath.Join(root, "alpha.md"), "# Alpha\n")
	writeNavTestFile(t, filepath.Join(root, ".pages"), "nav:\n  - missing.md\n  - alpha.md\n")

	nav, err := BuildTree(root, "")
	if err != nil {
		t.Fatal(err)
	}
	assertNavChildren(t, nav, []string{"alpha.md"})
}

func TestPagesCannotOverrideContentFilter(t *testing.T) {
	root := t.TempDir()
	writeNavTestFile(t, filepath.Join(root, "alpha.md"), "# Alpha\n")
	writeNavTestFile(t, filepath.Join(root, "hidden.md"), "# Hidden\n")
	writeNavTestFile(t, filepath.Join(root, "unlisted.md"), "# Unlisted\n")
	writeNavTestFile(t, filepath.Join(root, ".pages"), "nav:\n  - hidden.md\n  - alpha.md\n")

	nav, err := BuildTreeWithFilter(root, "", Filter{Exclude: []string{"hidden.md"}})
	if err != nil {
		t.Fatal(err)
	}
	assertNavChildren(t, nav, []string{"alpha.md", "unlisted.md"})
}

func assertNavChildren(t *testing.T, node *NavNode, want []string) {
	t.Helper()
	if len(node.Children) != len(want) {
		t.Fatalf("got %d children, want %d", len(node.Children), len(want))
	}
	for i, sourceName := range want {
		if node.Children[i].SourceName != sourceName {
			t.Fatalf("child %d = %q, want %q", i, node.Children[i].SourceName, sourceName)
		}
	}
}
