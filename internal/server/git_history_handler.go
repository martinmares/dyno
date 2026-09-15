package server

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// findGitRoot walks up from dir until it finds a .git directory, returns that directory or "".
func findGitRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// GitCommit holds a single parsed git log entry.
type GitCommit struct {
	Hash    string
	Author  string
	Date    string
	Message string
}

func (s *Server) gitHistoryHandler(w http.ResponseWriter, r *http.Request) {
	commits := s.loadGitHistory()
	contentHTML := renderGitHistoryHTML(commits)

	data := PageData{
		Title:         "Git history",
		ContentHTML:   template.HTML(contentHTML),
		Nav:           s.getNav(),
		IsHTMX:        isHTMX(r),
		LightCSS:      template.CSS(s.renderer.LightCSS()),
		DarkCSS:       template.CSS(s.renderer.DarkCSS()),
		Site:          s.siteCfg,
		BasePath:      s.basePath,
		AppCSSURL:     s.assetURL("app.css"),
		AppJSURL:      s.assetURL("app.js"),
		HTMXURL:       s.assetURL("htmx.min.js"),
		MermaidURL:    s.assetURL("mermaid.min.js"),
		FaviconURL:    s.faviconURL(),
		SearchURL:     s.searchPath,
		TasksURL:      s.tasksPath,
		GraphURL:      s.graphPath,
		GitHistoryURL: s.gitHistoryPath,
		BuildVersion:  s.version,
		BuildCommit:   s.commit,
	}

	if isHTMX(r) {
		w.Header().Set("HX-Push-Url", s.gitHistoryPath)
		if err := s.render(w, "page-fragment", data); err != nil {
			slog.Error("template error", "template", "page-fragment", "err", err)
		}
		return
	}
	if err := s.render(w, "base.html", data); err != nil {
		slog.Error("template error", "template", "base.html", "err", err)
	}
}

func renderGitHistoryHTML(commits []GitCommit) string {
	if len(commits) == 0 {
		return `<div class="dyno-history"><h1>Git history</h1><p class="dyno-history-summary">Git history is not available for this page.</p></div>`
	}

	var b strings.Builder
	b.WriteString(`<div class="dyno-history">`)
	fmt.Fprintf(&b, `<h1>Git history</h1>`)
	fmt.Fprintf(&b, `<p class="dyno-history-summary">Latest %d commits</p>`, len(commits))
	b.WriteString(`<div class="dyno-history-table-wrap">`)
	b.WriteString(`<table class="table table-vcenter dyno-history-table">`)
	b.WriteString(`<thead><tr>`)
	b.WriteString(`<th>Date</th>`)
	b.WriteString(`<th>Commit</th>`)
	b.WriteString(`<th>Author</th>`)
	b.WriteString(`<th>Message</th>`)
	b.WriteString(`</tr></thead><tbody>`)
	for _, c := range commits {
		fmt.Fprintf(&b,
			`<tr>`+
				`<td class="dyno-history-date">%s</td>`+
				`<td><code class="dyno-history-hash">%s</code></td>`+
				`<td class="dyno-history-author">%s</td>`+
				`<td>%s</td>`+
				`</tr>`,
			template.HTMLEscapeString(c.Date),
			template.HTMLEscapeString(c.Hash),
			template.HTMLEscapeString(c.Author),
			template.HTMLEscapeString(c.Message),
		)
	}
	b.WriteString(`</tbody></table></div></div>`)
	return b.String()
}

func (s *Server) loadGitHistory() []GitCommit {
	gitRoot := findGitRoot(s.contentDir)
	if gitRoot == "" {
		return nil
	}
	// format: hash|author|date|message — use \x1f as separator to handle commas in messages
	cmd := exec.Command("git", "-C", gitRoot, "log", "--max-count=50",
		"--pretty=format:%h\x1f%an\x1f%ad\x1f%s", "--date=format:%Y-%m-%d %H:%M:%S %z")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		slog.Warn("git log failed", "err", err, "siteRoot", s.siteRoot)
		return nil
	}

	var commits []GitCommit
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 4)
		if len(parts) != 4 {
			continue
		}
		commits = append(commits, GitCommit{
			Hash:    parts[0],
			Author:  parts[1],
			Date:    parts[2],
			Message: parts[3],
		})
	}
	return commits
}
