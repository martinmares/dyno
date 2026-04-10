package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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
	Prev        *navigation.NavNode
	Next        *navigation.NavNode
	TailwindURL string
	AppJSURL    string
	HTMXURL     string
	MermaidURL  string
	FaviconURL  string
}

// SearchData is passed to search templates.
type SearchData struct {
	Query       string
	Results     []search.SearchResult
	Nav         *navigation.NavNode
	IsHTMX      bool
	LightCSS    template.CSS
	DarkCSS     template.CSS
	Title       string
	TOC         []markdown.TOCEntry
	Site        *config.SiteConfig
	BasePath    string
	TailwindURL string
	AppJSURL    string
	HTMXURL     string
	FaviconURL  string
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

	info, err := os.Stat(fsPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.notFound(w, r)
			return
		}
		s.internalError(w, r, err)
		return
	}
	if !isHTMX(r) {
		pageETag := fmt.Sprintf(`W/"%x-%x"`, info.ModTime().UnixNano(), info.Size())
		w.Header().Set("ETag", pageETag)
		w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
		w.Header().Set("Cache-Control", "no-cache")
		if isConditionalRequestFresh(r, pageETag, info.ModTime()) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	res, err := s.getRenderedPage(fsPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.notFound(w, r)
			return
		}
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
		TailwindURL: s.assetURL("tailwind.css"),
		AppJSURL:    s.assetURL("app.js"),
		HTMXURL:     s.assetURL("htmx.min.js"),
		MermaidURL:  s.assetURL("mermaid.min.js"),
		FaviconURL:  s.faviconURL(),
	}

	if isHTMX(r) {
		pushURL := node.FullPath
		if rawQuery := r.URL.RawQuery; rawQuery != "" {
			pushURL += "?" + rawQuery
		}
		w.Header().Set("HX-Push-Url", pushURL)
		if err := s.render(w, "page-fragment", data); err != nil {
			slog.Error("template error", "template", "page-fragment", "err", err)
		}
		return
	}

	if err := s.render(w, "base.html", data); err != nil {
		slog.Error("template error", "template", "base.html", "err", err)
	}
}

func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var results []search.SearchResult
	if q != "" {
		results = s.getIdx().Search(q)
	}

	data := SearchData{
		Query:       q,
		Results:     results,
		Nav:         s.getNav(),
		IsHTMX:      isHTMX(r),
		LightCSS:    template.CSS(s.renderer.LightCSS()),
		DarkCSS:     template.CSS(s.renderer.DarkCSS()),
		Title:       "Search",
		Site:        s.siteCfg,
		BasePath:    s.basePath,
		TailwindURL: s.assetURL("tailwind.css"),
		AppJSURL:    s.assetURL("app.js"),
		HTMXURL:     s.assetURL("htmx.min.js"),
		FaviconURL:  s.faviconURL(),
	}

	if isHTMX(r) {
		if err := s.render(w, "search-results", data); err != nil {
			slog.Error("template error", "template", "search-results", "err", err)
		}
		return
	}

	if err := s.render(w, "search-page.html", data); err != nil {
		slog.Error("template error", "template", "search-page.html", "err", err)
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
		TailwindURL: s.assetURL("tailwind.css"),
		AppJSURL:    s.assetURL("app.js"),
		HTMXURL:     s.assetURL("htmx.min.js"),
		MermaidURL:  s.assetURL("mermaid.min.js"),
		FaviconURL:  s.faviconURL(),
	}
	tmplName := "base.html"
	if isHTMX(r) {
		tmplName = "page-content"
	}
	_ = s.render(w, tmplName, data)
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal error", "err", err, "path", r.URL.Path)
	w.WriteHeader(http.StatusInternalServerError)
	data := PageData{
		Title:       "Internal Error",
		ContentHTML: template.HTML(`<div class="text-center py-16"><h1 class="text-4xl font-bold text-red-400">500</h1><p class="mt-4 text-gray-500">Internal server error.</p></div>`),
		Nav:         s.getNav(),
		IsHTMX:      isHTMX(r),
		LightCSS:    template.CSS(s.renderer.LightCSS()),
		DarkCSS:     template.CSS(s.renderer.DarkCSS()),
		TailwindURL: s.assetURL("tailwind.css"),
		AppJSURL:    s.assetURL("app.js"),
		HTMXURL:     s.assetURL("htmx.min.js"),
		MermaidURL:  s.assetURL("mermaid.min.js"),
		FaviconURL:  s.faviconURL(),
	}
	tmplName := "base.html"
	if isHTMX(r) {
		tmplName = "page-content"
	}
	_ = s.render(w, tmplName, data)
}

// ── API proxy ──────────────────────────────────────────────────────────────

// apiProxyRequest is the JSON body sent by the browser JS.
type apiProxyRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

var apiHTTPClient = &http.Client{Timeout: 30 * time.Second}

func (s *Server) apiProxyHandler(w http.ResponseWriter, r *http.Request) {
	var req apiProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.URL == "" || req.Method == "" {
		http.Error(w, "method and url required", http.StatusBadRequest)
		return
	}
	if err := s.validateProxyTarget(r.Context(), req.URL); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<div class="api-error">Blocked target: %s</div>`, htmlEscapeStr(err.Error()))
		return
	}

	var bodyReader io.Reader
	if req.Body != "" {
		bodyReader = strings.NewReader(req.Body)
	}

	outReq, err := http.NewRequestWithContext(r.Context(), req.Method, req.URL, bodyReader)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<div class="api-error">Invalid URL: %s</div>`, htmlEscapeStr(err.Error()))
		return
	}
	for k, v := range req.Headers {
		outReq.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := apiHTTPClient.Do(outReq)
	elapsed := time.Since(start)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<div class="api-error">Request failed: %s</div>`, htmlEscapeStr(err.Error()))
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	ct := resp.Header.Get("Content-Type")

	// Pretty-print JSON if applicable.
	displayBody := string(respBody)
	if strings.Contains(ct, "application/json") || isJSON(respBody) {
		var v any
		if json.Unmarshal(respBody, &v) == nil {
			if pretty, err := json.MarshalIndent(v, "", "  "); err == nil {
				displayBody = string(pretty)
			}
		}
	}

	statusClass := "api-status-2xx"
	if resp.StatusCode >= 500 {
		statusClass = "api-status-5xx"
	} else if resp.StatusCode >= 400 {
		statusClass = "api-status-4xx"
	} else if resp.StatusCode >= 300 {
		statusClass = "api-status-3xx"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Return JSON payload so JS can populate both Body and Headers tabs.
	// Wrapped in a <script> + a hidden div carrying the data.
	payload := map[string]any{
		"status":      resp.StatusCode,
		"statusText":  resp.Status,
		"elapsed":     elapsed.Milliseconds(),
		"statusClass": statusClass,
		"body":        displayBody,
		"headers":     flattenHeaders(resp.Header),
	}
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, `<script>applyProxyResponse(%s)</script>`, data)
}

// flattenHeaders converts http.Header to ordered [][]string for JSON.
func flattenHeaders(h http.Header) [][2]string {
	out := make([][2]string, 0, len(h))
	for k, vs := range h {
		out = append(out, [2]string{k, strings.Join(vs, ", ")})
	}
	return out
}

func isJSON(b []byte) bool {
	b = []byte(strings.TrimSpace(string(b)))
	return len(b) > 0 && (b[0] == '{' || b[0] == '[')
}

func htmlEscapeStr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
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
