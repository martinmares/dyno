package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangeReportExcludesFrontmatterAndUnchangedContent(t *testing.T) {
	s := newTestServer(t)
	left := []byte("---\nsecret: old-secret\nstatus: NEW\n---\n# Guide\n\nUnchanged paragraph.\n\n## Permissions\n\nOld permission rule.\n")
	right := []byte("---\nsecret: new-secret\nstatus: REVISION\n---\n# Guide\n\nUnchanged paragraph.\n\n## Permissions\n\nNew permission rule.\n\n## Deployment\n\nA new deployment instruction.\n")
	changes, added, removed, changed, err := s.buildChangeReport(left, right, filepath.Join(s.contentDir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 || removed != 0 || changed != 1 || len(changes) != 3 {
		t.Fatalf("unexpected block counts: +%d -%d ~%d %+v", added, removed, changed, changes)
	}
	for _, change := range changes {
		content := string(change.Before) + string(change.After)
		for _, unwanted := range []string{"secret", "REVISION", "Unchanged paragraph"} {
			if strings.Contains(content, unwanted) {
				t.Fatalf("unrelated content leaked into report: %s", content)
			}
		}
	}
	if changes[0].Section != "Permissions" || !strings.Contains(string(changes[0].Before), "Old permission rule") || !strings.Contains(string(changes[0].After), "New permission rule") {
		t.Fatalf("missing change context: %+v", changes[0])
	}
	changes, _, _, _, err = s.buildChangeReport(left, []byte(strings.Replace(string(left), "old-secret", "new-secret", 1)), filepath.Join(s.contentDir, "index.md"))
	if err != nil || len(changes) != 0 {
		t.Fatalf("frontmatter-only change must yield empty report: %+v %v", changes, err)
	}
}

func TestChangeReportKeepsFencesTablesAndListsWhole(t *testing.T) {
	source := "# Guide\n\n## SQL\n\n```sql\nSELECT 1;\n\n# Not a heading\nSELECT 2;\n```\n\n## Rows\n\n| Name | Value |\n| --- | --- |\n| A | 1 |\n\n- First\n\n  More detail.\n\n- Second\n\nFinal paragraph.\n"
	blocks := changeReportBlocks([]byte(source))
	foundCode, foundTable, foundList := false, false, false
	for _, block := range blocks {
		switch block.Kind {
		case "Code block":
			foundCode = true
			if block.Section != "SQL" || !strings.Contains(block.Source, "SELECT 2;\n```") {
				t.Fatalf("fence/context truncated: %+v", block)
			}
		case "Table":
			foundTable = true
			if !strings.Contains(block.Source, "| A | 1 |") {
				t.Fatal("table split")
			}
		case "List":
			foundList = true
			if !strings.Contains(block.Source, "More detail.") || !strings.Contains(block.Source, "- Second") {
				t.Fatalf("loose list split: %+v", block)
			}
		}
	}
	if !foundCode || !foundTable || !foundList {
		t.Fatalf("missing block types: %+v", blocks)
	}
	s := newTestServer(t)
	changes, _, _, changed, err := s.buildChangeReport([]byte(source), []byte(strings.Replace(source, "SELECT 2;", "SELECT 3;", 1)), filepath.Join(s.contentDir, "index.md"))
	if err != nil || len(changes) != 1 || changed != 1 || changes[0].BlockType != "Code block" {
		t.Fatalf("single fenced edit must be one changed block: %+v %v", changes, err)
	}
}

func TestChangeReportHandlesAddedRemovedAndReferenceBlocks(t *testing.T) {
	s := newTestServer(t)
	for _, tt := range []struct {
		left, right    string
		added, removed int
	}{
		{"", "A new paragraph.", 1, 0},
		{"An old paragraph.", "", 0, 1},
		{"# Heading\n\nParagraph.", "# Heading\n\nParagraph.", 0, 0},
	} {
		changes, a, d, c, err := s.buildChangeReport([]byte(tt.left), []byte(tt.right), filepath.Join(s.contentDir, "index.md"))
		if err != nil || a != tt.added || d != tt.removed || c != 0 {
			t.Fatalf("unexpected changes %+v %v", changes, err)
		}
	}
	left := []byte("# Guide\n\nAn old [link][ref].\n\n[ref]: https://example.test/\n")
	right := []byte(strings.Replace(string(left), "old", "new", 1))
	changes, _, _, _, err := s.buildChangeReport(left, right, filepath.Join(s.contentDir, "index.md"))
	if err != nil || len(changes) != 1 || !strings.Contains(string(changes[0].After), `href="https://example.test/"`) {
		t.Fatalf("reference link lost context: %+v %v", changes, err)
	}
}

func TestChangeReportHeadingPreservesLiteralHash(t *testing.T) {
	blocks := changeReportBlocks([]byte("# Guide\n\n## C# ###\n\nExample.\n"))
	if len(blocks) != 3 || blocks[2].Section != "C#" {
		t.Fatalf("unexpected heading context: %+v", blocks)
	}
}

func TestGitChangeReportUsesSelectedDocumentRevisions(t *testing.T) {
	repo := initGitRepository(t)
	contentDir := filepath.Join(repo, "site")
	file := filepath.Join(contentDir, "guide.md")
	writeTestFile(t, filepath.Join(contentDir, "index.md"), "# Home\n")
	writeTestFile(t, file, "---\nprivate: before-secret\n---\n# Guide\n\nFirst public version.\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "first public version")
	writeTestFile(t, file, "---\nprivate: after-secret\n---\n# Guide\n\nSecond public version.\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "second public version")
	s := newGitTestServer(t, repo, contentDir)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_git/compare/guide?mode=report", nil))
	if rec.Code != 200 {
		t.Fatalf("report returned %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Change report", "First public version.", "Second public version.", "Print / Save PDF", "data-report-time", "Before:", "After:"} {
		if !strings.Contains(body, want) {
			t.Fatalf("report missing %q", want)
		}
	}
	for _, secret := range []string{"before-secret", "after-secret"} {
		if strings.Contains(body, secret) {
			t.Fatal("frontmatter leaked")
		}
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_git/compare/guide?mode=report&left=HEAD~100", nil))
	if rec.Code != 400 {
		t.Fatal("report accepted unlisted revision")
	}
}
