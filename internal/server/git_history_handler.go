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
		TailwindURL:   s.assetURL("tailwind.css"),
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
		return `<h1 class="text-2xl font-bold mb-4">Git history</h1><p class="text-gray-500 dark:text-gray-400">Git history není pro tuto stránku dostupná.</p>`
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<h1 class="text-2xl font-bold mb-2">Git history</h1>`)
	fmt.Fprintf(&b, `<p class="text-sm text-gray-500 dark:text-gray-400 mb-6">Posledních %d commitů</p>`, len(commits))
	b.WriteString(`<div class="overflow-x-auto not-prose">`)
	b.WriteString(`<table class="w-full text-sm border-collapse">`)
	b.WriteString(`<thead><tr class="border-b border-gray-200 dark:border-gray-700">`)
	b.WriteString(`<th class="text-left py-2 pr-4 font-semibold text-gray-600 dark:text-gray-400 whitespace-nowrap">Datum</th>`)
	b.WriteString(`<th class="text-left py-2 pr-4 font-semibold text-gray-600 dark:text-gray-400 whitespace-nowrap">Commit</th>`)
	b.WriteString(`<th class="text-left py-2 pr-4 font-semibold text-gray-600 dark:text-gray-400 whitespace-nowrap">Autor</th>`)
	b.WriteString(`<th class="text-left py-2 font-semibold text-gray-600 dark:text-gray-400">Zpráva</th>`)
	b.WriteString(`</tr></thead><tbody>`)
	for _, c := range commits {
		fmt.Fprintf(&b,
			`<tr class="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50">`+
				`<td class="py-2 pr-4 text-gray-500 dark:text-gray-400 whitespace-nowrap font-mono text-xs">%s</td>`+
				`<td class="py-2 pr-4 whitespace-nowrap"><code class="text-xs bg-gray-100 dark:bg-gray-800 text-brand-600 dark:text-brand-400 px-1.5 py-0.5 rounded font-mono">%s</code></td>`+
				`<td class="py-2 pr-4 text-gray-600 dark:text-gray-300 whitespace-nowrap">%s</td>`+
				`<td class="py-2 text-gray-800 dark:text-gray-200">%s</td>`+
				`</tr>`,
			template.HTMLEscapeString(c.Date),
			template.HTMLEscapeString(c.Hash),
			template.HTMLEscapeString(c.Author),
			template.HTMLEscapeString(c.Message),
		)
	}
	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

func (s *Server) loadGitHistory() []GitCommit {
	gitRoot := findGitRoot(s.contentDir)
	if gitRoot == "" {
		return nil
	}
	// format: hash|author|date|message — use \x1f as separator to handle commas in messages
	cmd := exec.Command("git", "-C", gitRoot, "log", "--max-count=50",
		"--pretty=format:%h\x1f%an\x1f%ad\x1f%s", "--date=format:%Y-%m-%d")
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
