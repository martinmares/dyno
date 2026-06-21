package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

func TestLoadGitFileRevisionsFollowsRename(t *testing.T) {
	repo := initGitRepository(t)
	oldPath := filepath.Join(repo, "docs", "old name.md")
	writeTestFile(t, oldPath, "# Document\n\nStable paragraph.\n\nOriginal ending.\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "add original")

	newPath := filepath.Join(repo, "docs", "renamed.md")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, newPath, "# Document\n\nStable paragraph.\n\nRenamed ending.\n")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-m", "rename document")
	writeTestFile(t, newPath, "# Working tree\n")

	revisions, err := loadGitFileRevisions(repo, "docs/renamed.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 {
		t.Fatalf("expected two revisions, got %#v", revisions)
	}
	if revisions[0].Path != "docs/renamed.md" || revisions[1].Path != "docs/old name.md" {
		t.Fatalf("rename paths were not followed: %#v", revisions)
	}
	content, err := loadGitRevision(repo, "docs/renamed.md", revisions[1])
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Document\n\nStable paragraph.\n\nOriginal ending.\n" {
		t.Fatalf("unexpected historical content %q", content)
	}
}

func TestGitCompareHandlerAndRevisionValidation(t *testing.T) {
	repo := initGitRepository(t)
	contentDir := filepath.Join(repo, "site")
	pagePath := filepath.Join(contentDir, "guide.md")
	writeTestFile(t, filepath.Join(contentDir, "index.md"), "# Home\n")
	writeTestFile(t, pagePath, "# Guide\n\nFirst version.\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "first version")
	writeTestFile(t, pagePath, "# Guide\n\nSecond version.\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "second version")
	writeTestFile(t, pagePath, "# Guide\n\nWorking version.\n")

	srv := newGitTestServer(t, repo, contentDir)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_git/compare/guide", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("compare returned %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{"Compare versions", "Working tree", "Second version.", "Working version.", "Preview", "dyno-theme"} {
		if !strings.Contains(body, expected) {
			t.Errorf("compare page does not contain %q", expected)
		}
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_git/compare/guide?left=HEAD~1&right=WORKTREE", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unlisted revision returned %d, want 400", rec.Code)
	}
}

func TestBuildSideBySideDiff(t *testing.T) {
	rows, added, deleted, changed := buildSideBySideDiff("same\nold\nremoved\n", "same\nnew\nadded\nextra\n")
	if added != 1 || deleted != 0 || changed != 2 {
		t.Fatalf("unexpected stats +%d -%d ~%d, rows=%#v", added, deleted, changed, rows)
	}
	if len(rows) != 4 || rows[0].Kind != "equal" || !rows[1].Changed {
		t.Fatalf("unexpected rows %#v", rows)
	}
}

func TestBuildGitSideBySideDiffKeepsShiftedLinesUnchanged(t *testing.T) {
	left := []byte("title: Example\ndocument-type: DetailDesign\n# Body\n")
	right := []byte("title: Example\nowner: team@example.test\ndocument-type: DetailDesign\n# Body\n")
	rows, added, deleted, changed, err := buildGitSideBySideDiff(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 || deleted != 0 || changed != 0 {
		t.Fatalf("unexpected stats +%d -%d ~%d, rows=%#v", added, deleted, changed, rows)
	}
	for _, row := range rows {
		if row.LeftText == "document-type: DetailDesign" && (row.Kind != "equal" || row.LeftLine != 2 || row.RightLine != 3) {
			t.Fatalf("shifted unchanged line was marked as changed: %#v", row)
		}
	}
}

func TestBuildGitSideBySideDiffKeepsFormerLastLineUnchanged(t *testing.T) {
	left := []byte("- Components: `Core`, `Infra`\n")
	right := []byte("- Components: `Core`, `Infra`\n\n## Implementation\nNew section.")
	rows, added, deleted, changed, err := buildGitSideBySideDiff(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if added != 3 || deleted != 0 || changed != 0 {
		t.Fatalf("unexpected stats +%d -%d ~%d, rows=%#v", added, deleted, changed, rows)
	}
	if len(rows) != 4 || rows[0].Kind != "equal" || rows[0].LeftText != rows[0].RightText {
		t.Fatalf("former last line was marked as changed: %#v", rows)
	}
}

func TestStripYAMLFrontmatter(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		want   string
	}{
		{name: "LF", source: "---\ntitle: Old\n---\n# Body\n", want: "# Body\n"},
		{name: "CRLF", source: "---\r\ntitle: Old\r\n---\r\n# Body\r\n", want: "# Body\r\n"},
		{name: "absent", source: "# Body\n", want: "# Body\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := string(stripYAMLFrontmatter([]byte(test.source))); got != test.want {
				t.Fatalf("stripYAMLFrontmatter() = %q, want %q", got, test.want)
			}
		})
	}
}

func initGitRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.name", "Dyno Test")
	runGit(t, repo, "config", "user.email", "dyno@example.test")
	return repo
}

func runGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func newGitTestServer(t *testing.T, siteRoot, contentDir string) *Server {
	t.Helper()
	cfg := &config.SiteConfig{Title: "Git Docs"}
	basePath := ""
	cfg.BasePath = &basePath
	cfg.Defaults()
	nav, err := navigation.BuildTree(contentDir, "")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	idx, err := search.BuildIndex(nav, func(src string) (string, error) { return renderer.ToPlainText([]byte(src)) })
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Config{SiteRoot: siteRoot, ContentDir: contentDir, DevMode: true, SiteCfg: cfg}, os.DirFS(repoRoot), nav, idx, renderer)
	if err != nil {
		t.Fatal(err)
	}
	return srv
}
