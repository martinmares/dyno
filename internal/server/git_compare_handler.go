package server

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
)

const (
	gitRevisionWorktree = "WORKTREE"
	gitRevisionIndex    = "INDEX"
	maxFileRevisions    = 100
)

type gitFileRevision struct {
	ID        string
	ShortHash string
	Author    string
	Date      string
	Message   string
	Path      string
	Label     string
}

type gitCompareRow struct {
	LeftLine  int
	LeftText  string
	RightLine int
	RightText string
	Kind      string
	Changed   bool
}

type gitCompareData struct {
	Title             string
	DocumentTitle     string
	PageURL           string
	CompareURL        string
	Revisions         []gitFileRevision
	Left              gitFileRevision
	Right             gitFileRevision
	SourceURL         string
	PreviewURL        string
	SwapURL           string
	Rows              []gitCompareRow
	LeftHTML          template.HTML
	RightHTML         template.HTML
	Mode              string
	IgnoreFrontmatter bool
	Added             int
	Deleted           int
	Changed           int
	TailwindURL       string
	AppCSSURL         string
	FaviconURL        string
	LightCSS          template.CSS
	DarkCSS           template.CSS
}

func (s *Server) gitCompareURLFor(pageFullPath string) string {
	if s.gitComparePath == "" || pageFullPath == "" {
		return ""
	}
	suffix := strings.TrimPrefix(pageFullPath, s.basePath)
	if suffix == "" {
		suffix = "/"
	}
	return s.gitComparePath + suffix
}

func (s *Server) gitCompareHandler(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, s.gitComparePath)
	pageURL := strings.TrimRight(s.basePath+suffix, "/")
	if pageURL == s.basePath {
		pageURL = s.basePath + "/"
	}
	node := navigation.FindNode(s.getNav(), pageURL)
	if node == nil {
		node = navigation.FindNode(s.getNav(), pageURL+"/")
	}
	if node == nil || node.FSPath == "" {
		s.notFound(w, r)
		return
	}

	repoRoot := findGitRoot(node.FSPath)
	relPath, ok := repositoryRelativePath(repoRoot, node.FSPath)
	if !ok {
		s.notFound(w, r)
		return
	}
	commits, err := loadGitFileRevisions(repoRoot, relPath)
	if err != nil || len(commits) == 0 {
		slog.Warn("load document git history", "path", relPath, "err", err)
		http.Error(w, "No Git revisions were found for this document.", http.StatusNotFound)
		return
	}

	revisions := make([]gitFileRevision, 0, len(commits)+2)
	revisions = append(revisions, gitFileRevision{ID: gitRevisionWorktree, Label: "Working tree"})
	if _, err := loadGitRevision(repoRoot, relPath, gitFileRevision{ID: gitRevisionIndex, Path: relPath}); err == nil {
		revisions = append(revisions, gitFileRevision{ID: gitRevisionIndex, Label: "Staged"})
	}
	revisions = append(revisions, commits...)

	leftID, rightID := defaultGitCompareRevisions(node.FSPath, commits)
	if value := strings.TrimSpace(r.URL.Query().Get("left")); value != "" {
		leftID = value
	}
	if value := strings.TrimSpace(r.URL.Query().Get("right")); value != "" {
		rightID = value
	}
	left, okLeft := findGitRevision(revisions, leftID)
	right, okRight := findGitRevision(revisions, rightID)
	if !okLeft || !okRight {
		http.Error(w, "The selected revision is not part of this document's history.", http.StatusBadRequest)
		return
	}
	leftSource, err := loadGitRevision(repoRoot, relPath, left)
	if err != nil {
		s.internalError(w, r, fmt.Errorf("load left revision: %w", err))
		return
	}
	rightSource, err := loadGitRevision(repoRoot, relPath, right)
	if err != nil {
		s.internalError(w, r, fmt.Errorf("load right revision: %w", err))
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode != "preview" {
		mode = "source"
	}
	data := gitCompareData{
		Title:             "Compare versions - " + node.Title,
		DocumentTitle:     node.Title,
		PageURL:           node.FullPath,
		CompareURL:        s.gitCompareURLFor(node.FullPath),
		Revisions:         revisions,
		Left:              left,
		Right:             right,
		Mode:              mode,
		IgnoreFrontmatter: r.URL.Query().Get("ignore_frontmatter") == "1",
		TailwindURL:       s.assetURL("tailwind.css"),
		AppCSSURL:         s.assetURL("app.css"),
		FaviconURL:        s.faviconURL(),
		LightCSS:          template.CSS(s.renderer.LightCSS()),
		DarkCSS:           template.CSS(s.renderer.DarkCSS()),
	}
	data.SourceURL = compareModeURL(data.CompareURL, left.ID, right.ID, "source", data.IgnoreFrontmatter)
	data.PreviewURL = compareModeURL(data.CompareURL, left.ID, right.ID, "preview", data.IgnoreFrontmatter)
	data.SwapURL = compareModeURL(data.CompareURL, right.ID, left.ID, mode, data.IgnoreFrontmatter)
	if mode == "preview" {
		data.LeftHTML, data.RightHTML, err = s.renderComparedMarkdown(leftSource, rightSource)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
	} else {
		if data.IgnoreFrontmatter {
			leftSource = stripYAMLFrontmatter(leftSource)
			rightSource = stripYAMLFrontmatter(rightSource)
		}
		data.Rows, data.Added, data.Deleted, data.Changed, err = buildGitSideBySideDiff(leftSource, rightSource)
		if err != nil {
			slog.Warn("git diff failed, using built-in diff", "err", err)
			data.Rows, data.Added, data.Deleted, data.Changed = buildSideBySideDiff(string(leftSource), string(rightSource))
		}
	}
	if err := s.render(w, "git-compare.html", data); err != nil {
		slog.Error("template error", "template", "git-compare.html", "err", err)
	}
}

func (s *Server) renderComparedMarkdown(left, right []byte) (template.HTML, template.HTML, error) {
	render := func(src []byte) (template.HTML, error) {
		result, err := s.renderer.RenderWithOptions(src, markdown.RenderOptions{TaskRenderer: s.renderTasks})
		if err != nil {
			return "", err
		}
		result.HTML = s.renderFileDownloads(result.HTML)
		return template.HTML(result.HTML), nil
	}
	leftHTML, err := render(left)
	if err != nil {
		return "", "", fmt.Errorf("render left revision: %w", err)
	}
	rightHTML, err := render(right)
	if err != nil {
		return "", "", fmt.Errorf("render right revision: %w", err)
	}
	return leftHTML, rightHTML, nil
}

func repositoryRelativePath(repoRoot, filePath string) (string, bool) {
	if repoRoot == "" {
		return "", false
	}
	rel, err := filepath.Rel(repoRoot, filePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func loadGitFileRevisions(repoRoot, currentPath string) ([]gitFileRevision, error) {
	cmd := exec.Command("git", "-C", repoRoot, "log", "--follow", "--find-renames=20%", "--name-status", "--date=iso-strict",
		"--max-count="+fmt.Sprint(maxFileRevisions), "--format=%x1e%H%x1f%h%x1f%an%x1f%ad%x1f%s", "--", currentPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	historicalPath := currentPath
	var revisions []gitFileRevision
	for _, record := range strings.Split(string(out), "\x1e") {
		record = strings.Trim(record, "\r\n")
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, "\n", 2)
		fields := strings.SplitN(strings.TrimSuffix(parts[0], "\r"), "\x1f", 5)
		if len(fields) != 5 {
			continue
		}
		date := formatGitRevisionDate(fields[3])
		revisions = append(revisions, gitFileRevision{
			ID: fields[0], ShortHash: fields[1], Author: fields[2], Date: date,
			Message: fields[4], Path: historicalPath,
			Label: fields[1] + " - " + date + " - " + fields[4],
		})
		if len(parts) == 2 {
			for _, statusLine := range strings.Split(parts[1], "\n") {
				status := strings.Split(strings.TrimSuffix(statusLine, "\r"), "\t")
				if len(status) == 3 && (strings.HasPrefix(status[0], "R") || strings.HasPrefix(status[0], "C")) && status[2] == historicalPath {
					historicalPath = status[1]
				}
			}
		}
	}
	return revisions, nil
}

func formatGitRevisionDate(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.Format("2006-01-02 15:04")
}

func loadGitRevision(repoRoot, currentPath string, revision gitFileRevision) ([]byte, error) {
	switch revision.ID {
	case gitRevisionWorktree:
		return os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(currentPath)))
	case gitRevisionIndex:
		return exec.Command("git", "-C", repoRoot, "show", ":"+currentPath).Output()
	default:
		if revision.Path == "" {
			return nil, fmt.Errorf("revision path is empty")
		}
		return exec.Command("git", "-C", repoRoot, "show", revision.ID+":"+revision.Path).Output()
	}
}

func defaultGitCompareRevisions(filePath string, commits []gitFileRevision) (string, string) {
	if gitFileStatus(filePath) != "" {
		return commits[0].ID, gitRevisionWorktree
	}
	if len(commits) > 1 {
		return commits[1].ID, commits[0].ID
	}
	return commits[0].ID, gitRevisionWorktree
}

func findGitRevision(revisions []gitFileRevision, id string) (gitFileRevision, bool) {
	for _, revision := range revisions {
		if revision.ID == id {
			return revision, true
		}
	}
	return gitFileRevision{}, false
}

var gitDiffHunkPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

type gitDiffHunk struct {
	leftStart  int
	leftCount  int
	rightStart int
	rightCount int
}

func buildGitSideBySideDiff(left, right []byte) ([]gitCompareRow, int, int, int, error) {
	leftLines := splitDiffLines(string(left))
	rightLines := splitDiffLines(string(right))

	tempDir, err := os.MkdirTemp("", "dyno-diff-")
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer os.RemoveAll(tempDir)
	leftPath := filepath.Join(tempDir, "left.md")
	rightPath := filepath.Join(tempDir, "right.md")
	// Normalize line endings so a CRLF/LF difference does not mark every line as changed.
	if err := os.WriteFile(leftPath, normalizedDiffSource(leftLines), 0o600); err != nil {
		return nil, 0, 0, 0, err
	}
	if err := os.WriteFile(rightPath, normalizedDiffSource(rightLines), 0o600); err != nil {
		return nil, 0, 0, 0, err
	}

	cmd := exec.Command("git", "diff", "--no-index", "--no-color", "--no-ext-diff", "--no-textconv",
		"--diff-algorithm=histogram", "--unified=0", "--", leftPath, rightPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 1 {
			return nil, 0, 0, 0, fmt.Errorf("git diff: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}

	hunks, err := parseGitDiffHunks(string(out))
	if err != nil {
		return nil, 0, 0, 0, err
	}
	rows := rowsFromGitDiffHunks(leftLines, rightLines, hunks)
	rows, added, deleted, changed := diffStats(rows)
	return rows, added, deleted, changed, nil
}

func normalizedDiffSource(lines []string) []byte {
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func parseGitDiffHunks(output string) ([]gitDiffHunk, error) {
	var hunks []gitDiffHunk
	for _, line := range strings.Split(output, "\n") {
		match := gitDiffHunkPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		leftStart, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, err
		}
		rightStart, err := strconv.Atoi(match[3])
		if err != nil {
			return nil, err
		}
		leftCount, rightCount := 1, 1
		if match[2] != "" {
			leftCount, err = strconv.Atoi(match[2])
			if err != nil {
				return nil, err
			}
		}
		if match[4] != "" {
			rightCount, err = strconv.Atoi(match[4])
			if err != nil {
				return nil, err
			}
		}
		hunks = append(hunks, gitDiffHunk{leftStart: leftStart, leftCount: leftCount, rightStart: rightStart, rightCount: rightCount})
	}
	return hunks, nil
}

func rowsFromGitDiffHunks(left, right []string, hunks []gitDiffHunk) []gitCompareRow {
	rows := make([]gitCompareRow, 0, max(len(left), len(right)))
	leftIndex, rightIndex := 0, 0
	appendEqual := func(leftEnd, rightEnd int) {
		for leftIndex < leftEnd && rightIndex < rightEnd {
			rows = append(rows, gitCompareRow{
				LeftLine: leftIndex + 1, LeftText: left[leftIndex],
				RightLine: rightIndex + 1, RightText: right[rightIndex], Kind: "equal",
			})
			leftIndex++
			rightIndex++
		}
	}
	for _, hunk := range hunks {
		leftStart := hunk.leftStart - 1
		if hunk.leftCount == 0 {
			leftStart = hunk.leftStart
		}
		rightStart := hunk.rightStart - 1
		if hunk.rightCount == 0 {
			rightStart = hunk.rightStart
		}
		appendEqual(leftStart, rightStart)
		count := max(hunk.leftCount, hunk.rightCount)
		for i := 0; i < count; i++ {
			row := gitCompareRow{Changed: true}
			if i < hunk.leftCount {
				row.LeftLine, row.LeftText = leftIndex+i+1, left[leftIndex+i]
			}
			if i < hunk.rightCount {
				row.RightLine, row.RightText = rightIndex+i+1, right[rightIndex+i]
			}
			switch {
			case i < hunk.leftCount && i < hunk.rightCount:
				row.Kind = "change"
			case i < hunk.leftCount:
				row.Kind = "delete"
			default:
				row.Kind = "add"
			}
			rows = append(rows, row)
		}
		leftIndex += hunk.leftCount
		rightIndex += hunk.rightCount
	}
	appendEqual(len(left), len(right))
	return rows
}

func buildSideBySideDiff(left, right string) ([]gitCompareRow, int, int, int) {
	leftLines := splitDiffLines(left)
	rightLines := splitDiffLines(right)
	// Bound memory for unusually large generated documents; regular documents use exact LCS alignment.
	if len(leftLines)*len(rightLines) > 4_000_000 {
		return positionalDiff(leftLines, rightLines)
	}
	dp := make([][]int, len(leftLines)+1)
	for i := range dp {
		dp[i] = make([]int, len(rightLines)+1)
	}
	for i := len(leftLines) - 1; i >= 0; i-- {
		for j := len(rightLines) - 1; j >= 0; j-- {
			if leftLines[i] == rightLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var rows []gitCompareRow
	i, j := 0, 0
	for i < len(leftLines) || j < len(rightLines) {
		if i < len(leftLines) && j < len(rightLines) && leftLines[i] == rightLines[j] {
			rows = append(rows, gitCompareRow{LeftLine: i + 1, LeftText: leftLines[i], RightLine: j + 1, RightText: rightLines[j], Kind: "equal"})
			i++
			j++
			continue
		}
		startI, startJ := i, j
		for (i < len(leftLines) || j < len(rightLines)) && !(i < len(leftLines) && j < len(rightLines) && leftLines[i] == rightLines[j]) {
			if j >= len(rightLines) || (i < len(leftLines) && dp[i+1][j] >= dp[i][j+1]) {
				i++
			} else {
				j++
			}
		}
		leftBlock, rightBlock := leftLines[startI:i], rightLines[startJ:j]
		count := max(len(leftBlock), len(rightBlock))
		for k := 0; k < count; k++ {
			row := gitCompareRow{Changed: true}
			if k < len(leftBlock) {
				row.LeftLine, row.LeftText = startI+k+1, leftBlock[k]
			}
			if k < len(rightBlock) {
				row.RightLine, row.RightText = startJ+k+1, rightBlock[k]
			}
			switch {
			case k < len(leftBlock) && k < len(rightBlock):
				row.Kind = "change"
			case k < len(leftBlock):
				row.Kind = "delete"
			default:
				row.Kind = "add"
			}
			rows = append(rows, row)
		}
	}
	return diffStats(rows)
}

func positionalDiff(left, right []string) ([]gitCompareRow, int, int, int) {
	rows := make([]gitCompareRow, 0, max(len(left), len(right)))
	for i := 0; i < max(len(left), len(right)); i++ {
		row := gitCompareRow{}
		if i < len(left) {
			row.LeftLine, row.LeftText = i+1, left[i]
		}
		if i < len(right) {
			row.RightLine, row.RightText = i+1, right[i]
		}
		switch {
		case i >= len(left):
			row.Kind, row.Changed = "add", true
		case i >= len(right):
			row.Kind, row.Changed = "delete", true
		case left[i] != right[i]:
			row.Kind, row.Changed = "change", true
		default:
			row.Kind = "equal"
		}
		rows = append(rows, row)
	}
	return diffStats(rows)
}

func diffStats(rows []gitCompareRow) ([]gitCompareRow, int, int, int) {
	added, deleted, changed := 0, 0, 0
	for _, row := range rows {
		switch row.Kind {
		case "add":
			added++
		case "delete":
			deleted++
		case "change":
			changed++
		}
	}
	return rows, added, deleted, changed
}

func splitDiffLines(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.TrimSuffix(value, "\n")
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

var yamlFrontmatterPattern = regexp.MustCompile(`(?s)^---\r?\n.*?\r?\n---(?:\r?\n)?`)

func stripYAMLFrontmatter(source []byte) []byte {
	return yamlFrontmatterPattern.ReplaceAll(source, nil)
}

func compareModeURL(compareURL, left, right, mode string, ignoreFrontmatter bool) string {
	values := url.Values{"left": {left}, "right": {right}, "mode": {mode}}
	if ignoreFrontmatter {
		values.Set("ignore_frontmatter", "1")
	}
	return compareURL + "?" + values.Encode()
}
