package server

import (
	"html/template"
	"log"
	"regexp"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

// PageData is passed to page templates.
type PageData struct {
	Title       string
	Breadcrumbs []navigation.NavNode
	ContentHTML template.HTML
	Nav         *navigation.NavNode
	CurrentPath string
	TOC         []markdown.TOCEntry
	LightCSS    template.CSS
	DarkCSS     template.CSS
	IsHTMX      bool
	Site        *config.SiteConfig
	BasePath    string
	// EditURL is the GitHub edit link for this page, empty if not configured
	EditURL string
	// Prev/Next for bottom navigation
	Prev *navigation.NavNode
	Next *navigation.NavNode
}

// SearchData is passed to search templates.
type SearchData struct {
	Query    string
	Results  []search.SearchResult
	Nav      *navigation.NavNode
	IsHTMX   bool
	LightCSS template.CSS
	DarkCSS  template.CSS
	Title    string
	TOC      []markdown.TOCEntry
	Site     *config.SiteConfig
	BasePath string
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// render executes a named template, re-parsing from disk in dev mode.
func (s *Server) render(w http.ResponseWriter, name string, data any) error {
	tmpl, err := s.getTemplate()
	if err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(w, name, data)
}

func (s *Server) pageHandler(w http.ResponseWriter, r *http.Request) {
	rawPath := r.PathValue("path")
	urlPath := s.basePath + "/" + rawPath

	// Serve static files (images, PDFs, etc.) from the site directory if the
	// path doesn't end with "/" and the file exists on disk.
	// Also tries prefixing the first path segment with "_" to support asset
	// directories like "_images/" referenced as "images/" in Markdown.
	if rawPath != "" && !strings.HasSuffix(rawPath, "/") {
		staticPath := filepath.Join(s.siteRoot, "site", filepath.FromSlash(rawPath))
		if info, err := os.Stat(staticPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, staticPath)
			return
		}
		// Try with "_" prefix on each directory segment to support asset
		// directories like "_images/" referenced as "images/" in Markdown.
		parts := strings.Split(rawPath, "/")
		for i := 0; i < len(parts)-1; i++ {
			if strings.HasPrefix(parts[i], "_") {
				continue // already prefixed
			}
			candidate := make([]string, len(parts))
			copy(candidate, parts)
			candidate[i] = "_" + candidate[i]
			altPath := filepath.Join(s.siteRoot, "site", filepath.Join(candidate...))
			if info, err := os.Stat(altPath); err == nil && !info.IsDir() {
				http.ServeFile(w, r, altPath)
				return
			}
		}
	}

	node := navigation.FindNode(s.getNav(), urlPath)
	if node == nil {
		node = navigation.FindNode(s.getNav(), strings.TrimRight(urlPath, "/"))
	}
	if node == nil {
		node = navigation.FindNode(s.getNav(), strings.TrimRight(urlPath, "/")+"/")
	}
	if node == nil {
		s.notFound(w, r)
		return
	}

	fsPath := node.FSPath
	if fsPath == "" {
		s.notFound(w, r)
		return
	}

	src, err := os.ReadFile(fsPath)
	if err != nil {
		s.notFound(w, r)
		return
	}

	res, err := s.renderer.Render(src)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	res.HTML = rewriteAbsoluteLinks(res.HTML, s.basePath)

	title := res.Title
	if title == "" {
		title = node.Title
	}

	prev, next := navigation.PrevNext(s.getNav(), node.FullPath)

	editURL := ""
	if s.siteCfg.GitHubURL != "" && node.FSPath != "" {
		// Build path relative to siteRoot
		rel, err := filepath.Rel(s.siteRoot, node.FSPath)
		if err == nil {
			editURL = s.siteCfg.GitHubURL + "/edit/" + s.siteCfg.GitHubBranch + "/" + filepath.ToSlash(rel)
		}
	}

	data := PageData{
		Title:       title,
		Breadcrumbs: navigation.Breadcrumbs(s.getNav(), node.FullPath),
		ContentHTML: template.HTML(res.HTML),
		Nav:         s.getNav(),
		CurrentPath: node.FullPath,
		TOC:         res.TOC,
		LightCSS:    template.CSS(s.renderer.LightCSS()),
		DarkCSS:     template.CSS(s.renderer.DarkCSS()),
		IsHTMX:      isHTMX(r),
		Site:        s.siteCfg,
		BasePath:    s.basePath,
		EditURL:     editURL,
		Prev:        prev,
		Next:        next,
	}

	if isHTMX(r) {
		w.Header().Set("HX-Push-Url", node.FullPath)
		if err := s.render(w, "page-content", data); err != nil {
			log.Printf("template error: %v", err)
		}
		return
	}

	if err := s.render(w, "base.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var results []search.SearchResult
	if q != "" {
		results = s.getIdx().Search(q)
	}

	data := SearchData{
		Query:    q,
		Results:  results,
		Nav:      s.getNav(),
		IsHTMX:   isHTMX(r),
		LightCSS: template.CSS(s.renderer.LightCSS()),
		DarkCSS:  template.CSS(s.renderer.DarkCSS()),
		Title:    "Search",
		Site:     s.siteCfg,
		BasePath: s.basePath,
	}

	if isHTMX(r) {
		if err := s.render(w, "search-results", data); err != nil {
			log.Printf("template error: %v", err)
		}
		return
	}

	if err := s.render(w, "search-page.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	data := PageData{
		Title:       "Page Not Found",
		ContentHTML: template.HTML(`<div class="text-center py-16"><h1 class="text-4xl font-bold text-gray-400">404</h1><p class="mt-4 text-gray-500">Page not found.</p></div>`),
		Nav:         s.getNav(),
		IsHTMX:      isHTMX(r),
		LightCSS:    template.CSS(s.renderer.LightCSS()),
		DarkCSS:     template.CSS(s.renderer.DarkCSS()),
	}
	tmplName := "base.html"
	if isHTMX(r) {
		tmplName = "page-content"
	}
	_ = s.render(w, tmplName, data)
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("internal error: %v", err)
	w.WriteHeader(http.StatusInternalServerError)
	data := PageData{
		Title:       "Internal Error",
		ContentHTML: template.HTML(`<div class="text-center py-16"><h1 class="text-4xl font-bold text-red-400">500</h1><p class="mt-4 text-gray-500">Internal server error.</p></div>`),
		Nav:         s.getNav(),
		IsHTMX:      isHTMX(r),
		LightCSS:    template.CSS(s.renderer.LightCSS()),
		DarkCSS:     template.CSS(s.renderer.DarkCSS()),
	}
	tmplName := "base.html"
	if isHTMX(r) {
		tmplName = "page-content"
	}
	_ = s.render(w, tmplName, data)
}

// absLinkRe matches href="/..." and src="/..." that are not protocol-relative (//...)
var absLinkRe = regexp.MustCompile(`(href|src)="(/[^/][^"]*)"`)

// rewriteAbsoluteLinks prefixes absolute paths (e.g. /getting-started/) with
// basePath (e.g. /docs), so authors never need to include basePath in their links.
// Paths already starting with basePath are left untouched.
func rewriteAbsoluteLinks(htmlStr, basePath string) string {
	if basePath == "" || basePath == "/" {
		return htmlStr
	}
	return absLinkRe.ReplaceAllStringFunc(htmlStr, func(match string) string {
		subs := absLinkRe.FindStringSubmatch(match)
		attr, path := subs[1], subs[2]
		// Don't double-prefix
		if strings.HasPrefix(path, basePath+"/") || path == basePath {
			return match
		}
		return attr + `="` + basePath + path + `"`
	})
}

