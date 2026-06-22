package server

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mares/dyno/internal/comments"
	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/metadata"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

// PageData is passed to page templates.
type PageData struct {
	Title             string
	Breadcrumbs       []navigation.NavNode
	ContentHTML       template.HTML
	Nav               *navigation.NavNode
	CurrentPath       string
	TOC               []markdown.TOCEntry
	ExternalRefs      []markdown.ExternalRef
	Comments          []comments.Comment
	Highlights        []comments.Comment
	CommentDocumentID string
	CommentTree       []CommentNode
	CommentsURL       string
	LightCSS          template.CSS
	DarkCSS           template.CSS
	IsHTMX            bool
	Site              *config.SiteConfig
	BasePath          string
	// EditURL is the GitHub edit link for this page, empty if not configured
	EditURL    string
	IsAgentDoc bool
	// Prev/Next for bottom navigation
	Prev              *navigation.NavNode
	Next              *navigation.NavNode
	TailwindURL       string
	AppCSSURL         string
	AppJSURL          string
	EasyMDECSSURL     string
	EasyMDEJSURL      string
	FontAwesomeURL    string
	HTMXURL           string
	MermaidURL        string
	FaviconURL        string
	SearchURL         string
	TasksURL          string
	SectionTasksURL   string // URL for section-scoped task view (current folder)
	HasTasksBlock     bool
	TasksAnchorURL    string
	Backlinks         []navigation.Backlink // pages that link to this page
	GraphURL          string                // URL for the site link graph page
	EgoGraphURL       string                // URL for ego-graph of current page
	LibraryURL        string                // non-empty in library mode: URL back to the dashboard
	BuildVersion      string
	BuildCommit       string
	EditMode          bool   // true when --edit is active
	EditPageURL       string // URL to open the editor for this page (empty if not editable)
	GitHistoryURL     string // non-empty when git history is available
	GitCompareURL     string // compare revisions of the current document
	MetadataFacets    []metadata.Facet
	FilterQuery       string
	FiltersActive     bool
	MetadataFilterURL string
}

type CommentNode struct {
	Comment  comments.Comment
	BodyHTML template.HTML
	Children []CommentNode
	Depth    int
}

// LibraryData is passed to the library dashboard template.
type LibraryData struct {
	Title          string // page <title>
	LogoText       string // short name shown in navbar, e.g. "Dyno"
	Subtitle       string
	Books          []bookCardData
	BasePath       string
	SearchURL      string
	TailwindURL    string
	AppCSSURL      string
	AppJSURL       string
	HTMXURL        string
	MermaidURL     string
	LightCSS       template.CSS
	DarkCSS        template.CSS
	IsHTMX         bool
	MetadataFacets []metadata.Facet
	FilterQuery    string
	FiltersActive  bool
}

// SearchData is passed to search templates.
type SearchData struct {
	Query          string
	Results        []search.SearchResult
	Nav            *navigation.NavNode
	IsHTMX         bool
	LightCSS       template.CSS
	DarkCSS        template.CSS
	Title          string
	TOC            []markdown.TOCEntry
	Site           *config.SiteConfig
	BasePath       string
	TailwindURL    string
	AppCSSURL      string
	AppJSURL       string
	HTMXURL        string
	FaviconURL     string
	SearchURL      string
	TasksURL       string
	GraphURL       string
	BuildVersion   string
	BuildCommit    string
	MetadataFacets []metadata.Facet
	FilterQuery    string
	FiltersActive  bool
	ResultQuery    string
}

type metadataView struct {
	Nav     *navigation.NavNode
	Facets  []metadata.Facet
	Query   string
	Allowed map[string]bool
	Active  bool
}

func (s *Server) metadataView(r *http.Request) metadataView {
	idx := s.getMetadata()
	if idx == nil || !idx.Enabled() {
		return metadataView{Nav: s.getNav()}
	}
	filter := idx.Parse(r.URL.Query())
	allowed := idx.Allowed(filter)
	return metadataView{
		Nav: navigation.FilterTree(s.getNav(), allowed), Facets: idx.Facets(filter),
		Query: metadata.Encode(filter), Allowed: allowed, Active: len(filter) > 0,
	}
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// render executes a named template, re-parsing from disk in dev mode.
func (s *Server) render(w http.ResponseWriter, name string, data any) error {
	if page, ok := data.(PageData); ok {
		if s.comments != nil {
			page.EasyMDECSSURL = s.assetURL("easymde.min.css")
			page.EasyMDEJSURL = s.assetURL("easymde.min.js")
			page.FontAwesomeURL = s.assetURL("font-awesome.min.css")
		}
		data = page
	}
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
		staticPath := filepath.Join(s.contentDir, filepath.FromSlash(rawPath))
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
			altPath := filepath.Join(s.contentDir, filepath.Join(candidate...))
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
	view := s.metadataView(r)

	fsPath := node.FSPath
	if fsPath == "" && node.IsDir {
		s.renderSyntheticIndex(w, r, node)
		return
	}
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
	if !isHTMX(r) && s.comments == nil && !view.Active {
		pageETag := fmt.Sprintf(`W/"%x-%x"`, info.ModTime().UnixNano(), info.Size())
		w.Header().Set("ETag", pageETag)
		w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
		w.Header().Set("Cache-Control", "no-cache")
		if isConditionalRequestFresh(r, pageETag, info.ModTime()) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	} else if !isHTMX(r) {
		w.Header().Set("Cache-Control", "no-store")
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
	res.HTML = s.rewriteMDLinks(res.HTML, node.FSPath)
	res.HTML = rewriteAbsoluteLinks(res.HTML, s.basePath)

	title := res.Title
	if title == "" {
		title = node.Title
	}

	prev, next := navigation.PrevNext(view.Nav, node.FullPath)
	commentDocumentID := s.commentDocumentID(node.FullPath)
	pageComments, err := s.listComments(commentDocumentID, node.FullPath)
	if err != nil {
		slog.Warn("failed to load comments", "path", node.FullPath, "err", err)
	}

	editURL := ""
	if s.siteCfg.GitHubURL != "" && node.FSPath != "" {
		// Build path relative to siteRoot
		rel, err := filepath.Rel(s.siteRoot, node.FSPath)
		if err == nil {
			editURL = s.siteCfg.GitHubURL + "/edit/" + s.siteCfg.GitHubBranch + "/" + filepath.ToSlash(rel)
		}
	}

	visibleComments := make([]comments.Comment, 0, len(pageComments))
	highlights := make([]comments.Comment, 0)
	for _, item := range pageComments {
		if item.Kind == "highlight" {
			highlights = append(highlights, item)
		} else {
			visibleComments = append(visibleComments, item)
		}
	}
	commentsURL := s.commentsPath
	if commentDocumentID == "" {
		commentsURL = ""
	}
	data := PageData{
		Title:             title,
		Breadcrumbs:       navigation.Breadcrumbs(s.getNav(), node.FullPath),
		ContentHTML:       template.HTML(res.HTML),
		Nav:               view.Nav,
		CurrentPath:       node.FullPath,
		TOC:               res.TOC,
		ExternalRefs:      res.Frontmatter.ExternalRefs,
		Comments:          visibleComments,
		Highlights:        highlights,
		CommentTree:       buildCommentTree(visibleComments, renderCommentMarkdown),
		CommentsURL:       commentsURL,
		CommentDocumentID: commentDocumentID,
		LightCSS:          template.CSS(s.renderer.LightCSS()),
		DarkCSS:           template.CSS(s.renderer.DarkCSS()),
		IsHTMX:            isHTMX(r),
		Site:              s.siteCfg,
		BasePath:          s.basePath,
		EditURL:           editURL,
		IsAgentDoc:        filepath.Base(node.FSPath) == "AGENTS.md",
		Prev:              prev,
		Next:              next,
		TailwindURL:       s.assetURL("tailwind.css"),
		AppCSSURL:         s.assetURL("app.css"),
		AppJSURL:          s.assetURL("app.js"),
		HTMXURL:           s.assetURL("htmx.min.js"),
		MermaidURL:        s.assetURL("mermaid.min.js"),
		FaviconURL:        s.faviconURL(),
		SearchURL:         s.searchPath + view.Query,
		TasksURL:          s.tasksPath,
		GraphURL:          s.graphPath,
		SectionTasksURL:   s.sectionTaskPath + "?path=" + node.FullPath,
		HasTasksBlock:     s.taskIndex.HasTasksBlock(node.FullPath),
		Backlinks:         s.backlinks[node.FullPath],
		EgoGraphURL:       s.graphPath + strings.TrimPrefix(node.FullPath, s.basePath) + view.Query,
		LibraryURL:        s.libraryURL,
		BuildVersion:      s.version,
		BuildCommit:       s.commit,
		EditMode:          s.editMode,
		EditPageURL: func() string {
			if s.editMode && node.FSPath != "" {
				return s.editPageURLFor(node.FullPath)
			}
			return ""
		}(),
		GitHistoryURL:     s.gitHistoryPath,
		GitCompareURL:     s.gitCompareURLFor(node.FullPath),
		MetadataFacets:    view.Facets,
		FilterQuery:       view.Query,
		FiltersActive:     view.Active,
		MetadataFilterURL: node.FullPath,
	}
	if data.HasTasksBlock {
		data.TasksAnchorURL = node.FullPath + "#tasks-0-filter"
	}

	if isHTMX(r) {
		pushURL := node.FullPath
		if rawQuery := r.URL.RawQuery; rawQuery != "" {
			pushURL += "?" + rawQuery
		}
		w.Header().Set("HX-Push-Url", pushURL)
		w.Header().Set("X-Search-URL", s.searchPath+view.Query)
		if err := s.render(w, "page-fragment", data); err != nil {
			slog.Error("template error", "template", "page-fragment", "err", err)
		}
		return
	}

	if err := s.render(w, "base.html", data); err != nil {
		slog.Error("template error", "template", "base.html", "err", err)
	}
}

func (s *Server) renderSyntheticIndex(w http.ResponseWriter, r *http.Request, node *navigation.NavNode) {
	view := s.metadataView(r)
	if filtered := navigation.FindNode(view.Nav, node.FullPath); filtered != nil {
		node = filtered
	}
	title := node.Title
	if title == "" || title == "Home" {
		if s.siteCfg != nil && s.siteCfg.Title != "" {
			title = s.siteCfg.Title
		} else {
			title = "Overview"
		}
	}

	var list strings.Builder
	list.WriteString(`<ul>`)
	for _, child := range node.Children {
		list.WriteString(`<li><a href="`)
		list.WriteString(child.FullPath)
		list.WriteString(view.Query)
		list.WriteString(`">`)
		list.WriteString(template.HTMLEscapeString(child.Title))
		list.WriteString(`</a></li>`)
	}
	list.WriteString(`</ul>`)

	body := `<p class="synthetic-index-note">` +
		`This section has no <code>index.md</code>, <code>README.md</code>, or <code>_group.md</code>, so this overview was generated automatically.` +
		`</p>`
	if len(node.Children) > 0 {
		body += `<h2>Pages in this section</h2>` + list.String()
	} else {
		body += `<p>No child pages were found in this section.</p>`
	}

	data := PageData{
		Title:             title,
		Breadcrumbs:       navigation.Breadcrumbs(s.getNav(), node.FullPath),
		ContentHTML:       template.HTML(body),
		Nav:               view.Nav,
		CurrentPath:       node.FullPath,
		TOC:               nil,
		LightCSS:          template.CSS(s.renderer.LightCSS()),
		DarkCSS:           template.CSS(s.renderer.DarkCSS()),
		IsHTMX:            isHTMX(r),
		Site:              s.siteCfg,
		BasePath:          s.basePath,
		IsAgentDoc:        false,
		TailwindURL:       s.assetURL("tailwind.css"),
		AppCSSURL:         s.assetURL("app.css"),
		AppJSURL:          s.assetURL("app.js"),
		HTMXURL:           s.assetURL("htmx.min.js"),
		MermaidURL:        s.assetURL("mermaid.min.js"),
		FaviconURL:        s.faviconURL(),
		SearchURL:         s.searchPath + view.Query,
		TasksURL:          s.tasksPath,
		GraphURL:          s.graphPath,
		GitHistoryURL:     s.gitHistoryPath,
		HasTasksBlock:     false,
		LibraryURL:        s.libraryURL,
		BuildVersion:      s.version,
		BuildCommit:       s.commit,
		MetadataFacets:    view.Facets,
		FilterQuery:       view.Query,
		FiltersActive:     view.Active,
		MetadataFilterURL: node.FullPath,
	}

	if isHTMX(r) {
		w.Header().Set("HX-Push-Url", node.FullPath+view.Query)
		w.Header().Set("X-Search-URL", s.searchPath+view.Query)
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
	start := time.Now()
	defer func() {
		s.metrics.searchRequests.Inc()
		s.metrics.searchDuration.Observe(time.Since(start).Seconds())
	}()

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	view := s.metadataView(r)
	resultValues := r.URL.Query()
	resultValues.Set("q", q)
	var results []search.SearchResult
	if q != "" {
		results = s.getIdx().SearchFiltered(q, view.Allowed)
	}

	data := SearchData{
		Query:          q,
		Results:        results,
		Nav:            view.Nav,
		IsHTMX:         isHTMX(r),
		LightCSS:       template.CSS(s.renderer.LightCSS()),
		DarkCSS:        template.CSS(s.renderer.DarkCSS()),
		Title:          "Search",
		Site:           s.siteCfg,
		BasePath:       s.basePath,
		TailwindURL:    s.assetURL("tailwind.css"),
		AppCSSURL:      s.assetURL("app.css"),
		AppJSURL:       s.assetURL("app.js"),
		HTMXURL:        s.assetURL("htmx.min.js"),
		FaviconURL:     s.faviconURL(),
		SearchURL:      s.searchPath + view.Query,
		TasksURL:       s.tasksPath,
		GraphURL:       s.graphPath,
		BuildVersion:   s.version,
		BuildCommit:    s.commit,
		MetadataFacets: view.Facets,
		FilterQuery:    view.Query,
		FiltersActive:  view.Active,
		ResultQuery:    "?" + resultValues.Encode(),
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
		Title:         "Page Not Found",
		ContentHTML:   template.HTML(`<div class="text-center py-16"><h1 class="text-4xl font-bold text-gray-400">404</h1><p class="mt-4 text-gray-500">Page not found.</p></div>`),
		Nav:           s.getNav(),
		IsHTMX:        isHTMX(r),
		LightCSS:      template.CSS(s.renderer.LightCSS()),
		DarkCSS:       template.CSS(s.renderer.DarkCSS()),
		TailwindURL:   s.assetURL("tailwind.css"),
		AppCSSURL:     s.assetURL("app.css"),
		AppJSURL:      s.assetURL("app.js"),
		HTMXURL:       s.assetURL("htmx.min.js"),
		MermaidURL:    s.assetURL("mermaid.min.js"),
		FaviconURL:    s.faviconURL(),
		SearchURL:     s.searchPath,
		TasksURL:      s.tasksPath,
		GraphURL:      s.graphPath,
		HasTasksBlock: false,
		BuildVersion:  s.version,
		BuildCommit:   s.commit,
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
		Title:         "Internal Error",
		ContentHTML:   template.HTML(`<div class="text-center py-16"><h1 class="text-4xl font-bold text-red-400">500</h1><p class="mt-4 text-gray-500">Internal server error.</p></div>`),
		Nav:           s.getNav(),
		IsHTMX:        isHTMX(r),
		LightCSS:      template.CSS(s.renderer.LightCSS()),
		DarkCSS:       template.CSS(s.renderer.DarkCSS()),
		TailwindURL:   s.assetURL("tailwind.css"),
		AppCSSURL:     s.assetURL("app.css"),
		AppJSURL:      s.assetURL("app.js"),
		HTMXURL:       s.assetURL("htmx.min.js"),
		MermaidURL:    s.assetURL("mermaid.min.js"),
		FaviconURL:    s.faviconURL(),
		SearchURL:     s.searchPath,
		TasksURL:      s.tasksPath,
		GraphURL:      s.graphPath,
		HasTasksBlock: false,
		BuildVersion:  s.version,
		BuildCommit:   s.commit,
	}
	tmplName := "base.html"
	if isHTMX(r) {
		tmplName = "page-content"
	}
	_ = s.render(w, tmplName, data)
}

func (s *Server) tasksHandler(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		Title:         "Task list summary",
		Breadcrumbs:   []navigation.NavNode{{Title: "Home", FullPath: s.basePath + "/"}, {Title: "Task list summary", FullPath: s.tasksPath}},
		ContentHTML:   template.HTML(s.taskHTML),
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
		HasTasksBlock: false,
		BuildVersion:  s.version,
		BuildCommit:   s.commit,
	}

	if isHTMX(r) {
		w.Header().Set("HX-Push-Url", s.tasksPath)
		if err := s.render(w, "page-fragment", data); err != nil {
			slog.Error("template error", "template", "page-fragment", "err", err)
		}
		return
	}
	if err := s.render(w, "base.html", data); err != nil {
		slog.Error("template error", "template", "base.html", "err", err)
	}
}

func (s *Server) sectionTasksHandler(w http.ResponseWriter, r *http.Request) {
	pagePath := r.URL.Query().Get("path")
	// Derive section prefix: parent folder of the given page path
	sectionPrefix := path.Dir(pagePath)
	if sectionPrefix == "." || sectionPrefix == "" {
		sectionPrefix = s.basePath
	}
	sectionLabel := path.Base(sectionPrefix)
	if sectionLabel == "." || sectionLabel == "" {
		sectionLabel = "Site"
	}
	selfURL := s.sectionTaskPath + "?path=" + pagePath
	contentHTML := s.taskIndex.RenderSection(sectionPrefix, "tasks-section")
	data := PageData{
		Title:           "Tasks — " + sectionLabel,
		Breadcrumbs:     []navigation.NavNode{{Title: "Home", FullPath: s.basePath + "/"}, {Title: "All tasks", FullPath: s.tasksPath}, {Title: sectionLabel + " tasks", FullPath: selfURL}},
		ContentHTML:     template.HTML(contentHTML),
		Nav:             s.getNav(),
		IsHTMX:          isHTMX(r),
		LightCSS:        template.CSS(s.renderer.LightCSS()),
		DarkCSS:         template.CSS(s.renderer.DarkCSS()),
		Site:            s.siteCfg,
		BasePath:        s.basePath,
		TailwindURL:     s.assetURL("tailwind.css"),
		AppCSSURL:       s.assetURL("app.css"),
		AppJSURL:        s.assetURL("app.js"),
		HTMXURL:         s.assetURL("htmx.min.js"),
		MermaidURL:      s.assetURL("mermaid.min.js"),
		FaviconURL:      s.faviconURL(),
		SearchURL:       s.searchPath,
		TasksURL:        s.tasksPath,
		GraphURL:        s.graphPath,
		SectionTasksURL: selfURL,
		HasTasksBlock:   false,
		BuildVersion:    s.version,
		BuildCommit:     s.commit,
	}
	if isHTMX(r) {
		w.Header().Set("HX-Push-Url", selfURL)
		if err := s.render(w, "page-fragment", data); err != nil {
			slog.Error("template error", "template", "page-fragment", "err", err)
		}
		return
	}
	if err := s.render(w, "base.html", data); err != nil {
		slog.Error("template error", "template", "base.html", "err", err)
	}
}

// graphHandler renders a dependency table for the whole site.
func (s *Server) graphHandler(w http.ResponseWriter, r *http.Request) {
	contentHTML := template.HTML(buildDepsTable(s.getNav(), s.backlinks, s.graphPath))
	selfURL := s.graphPath
	data := PageData{
		Title:        "Page Dependencies",
		Breadcrumbs:  []navigation.NavNode{{Title: "Home", FullPath: s.basePath + "/"}, {Title: "Page Dependencies", FullPath: selfURL}},
		ContentHTML:  contentHTML,
		Nav:          s.getNav(),
		IsHTMX:       isHTMX(r),
		LightCSS:     template.CSS(s.renderer.LightCSS()),
		DarkCSS:      template.CSS(s.renderer.DarkCSS()),
		Site:         s.siteCfg,
		BasePath:     s.basePath,
		TailwindURL:  s.assetURL("tailwind.css"),
		AppCSSURL:    s.assetURL("app.css"),
		AppJSURL:     s.assetURL("app.js"),
		HTMXURL:      s.assetURL("htmx.min.js"),
		MermaidURL:   s.assetURL("mermaid.min.js"),
		FaviconURL:   s.faviconURL(),
		SearchURL:    s.searchPath,
		TasksURL:     s.tasksPath,
		GraphURL:     s.graphPath,
		BuildVersion: s.version,
		BuildCommit:  s.commit,
	}
	if isHTMX(r) {
		w.Header().Set("HX-Push-Url", selfURL)
		if err := s.render(w, "page-fragment", data); err != nil {
			slog.Error("template error", "template", "page-fragment", "err", err)
		}
		return
	}
	if err := s.render(w, "base.html", data); err != nil {
		slog.Error("template error", "template", "base.html", "err", err)
	}
}

// egoGraphHandler renders a D2 ego-graph (±1 hop) as a full page.
// Route: GET /_graph/{page-path...} where page-path mirrors the page URL segments.
func (s *Server) egoGraphHandler(w http.ResponseWriter, r *http.Request) {
	view := s.metadataView(r)
	// Reconstruct the page URL from the path suffix after /_graph
	suffix := strings.TrimPrefix(r.URL.Path, s.graphPath)
	pagePath := suffix // e.g. /bss/crm/crm → we need full page URL: basePath+suffix
	if !strings.HasPrefix(pagePath, s.basePath) {
		pagePath = s.basePath + pagePath
	}
	pagePath = strings.TrimRight(pagePath, "/")

	// Find node in nav
	var centerNode *navigation.NavNode
	navigation.WalkNodes(s.getNav(), func(n *navigation.NavNode) {
		if strings.TrimRight(n.FullPath, "/") == pagePath {
			centerNode = n
		}
	})
	if centerNode == nil {
		s.notFound(w, r)
		return
	}

	// Build outgoing links: pages this page links to
	outgoing := map[string]navigation.Backlink{}
	for target, sources := range s.backlinks {
		for _, src := range sources {
			if strings.TrimRight(src.FullPath, "/") == pagePath {
				outgoing[target] = navigation.Backlink{Title: src.Title, FullPath: target}
			}
		}
	}
	// Find titles for outgoing targets
	navigation.WalkNodes(s.getNav(), func(n *navigation.NavNode) {
		if bl, ok := outgoing[n.FullPath]; ok {
			bl.Title = n.Title
			outgoing[n.FullPath] = bl
		}
	})

	incoming := s.backlinks[centerNode.FullPath]

	// Build ±1 hop ego-graph
	type egoNode struct{ url, title string }
	nodes := map[string]egoNode{pagePath: {pagePath, centerNode.Title}}
	for _, bl := range incoming {
		nodes[bl.FullPath] = egoNode{bl.FullPath, bl.Title}
	}
	for url, bl := range outgoing {
		nodes[url] = egoNode{url, bl.Title}
	}

	var d2 strings.Builder
	d2.WriteString("direction: right\n\n")
	for _, n := range nodes {
		label := `"` + strings.ReplaceAll(n.title, `"`, `'`) + `"`
		id := egoNodeID(n.url)
		link := n.url + view.Query
		fmt.Fprintf(&d2, "%s: %s {\n  link: \"%s\"\n}\n", id, label, strings.ReplaceAll(link, `"`, `'`))
	}
	d2.WriteString("\n")
	for _, bl := range incoming {
		fmt.Fprintf(&d2, "%s -> %s\n", egoNodeID(bl.FullPath), egoNodeID(pagePath))
	}
	for url := range outgoing {
		fmt.Fprintf(&d2, "%s -> %s\n", egoNodeID(pagePath), egoNodeID(url))
	}

	svg, err := markdown.RenderD2(d2.String())
	if err != nil {
		s.internalError(w, r, fmt.Errorf("ego-graph render: %w", err))
		return
	}

	// Build neighbour list with HTMX [+] expand buttons
	neighboursURL := s.graphPath + "/_neighbours"
	var content strings.Builder
	content.WriteString(svg)
	if len(nodes) > 1 {
		content.WriteString(`<div class="ego-neighbours">`)
		content.WriteString(`<p class="ego-neighbours-heading">Neighbours</p>`)
		for nodeURL, n := range nodes {
			if nodeURL == pagePath {
				continue
			}
			expandID := "nb-" + egoNodeID(nodeURL)
			nbValues := url.Values{"path": {nodeURL}, "exclude": {pagePath}}
			nbURL := appendFilterQuery(neighboursURL+"?"+nbValues.Encode(), view.Query)
			pageURL := nodeURL + view.Query
			content.WriteString(`<div class="ego-nb-row">`)
			content.WriteString(`<a class="ego-nb-title" href="` + html.EscapeString(pageURL) + `" hx-get="` + html.EscapeString(pageURL) + `" hx-target="#page-content" hx-push-url="true">` + html.EscapeString(n.title) + `</a>`)
			content.WriteString(`<button class="ego-nb-expand" hx-get="` + html.EscapeString(nbURL) + `" hx-target="#` + expandID + `" hx-swap="innerHTML" hx-indicator="#` + expandID + `-ind" onclick="this.style.display='none'">+</button>`)
			content.WriteString(`<span id="` + expandID + `-ind" class="htmx-indicator ego-nb-loading">…</span>`)
			content.WriteString(`<div id="` + expandID + `" class="ego-nb-children"></div>`)
			content.WriteString(`</div>`)
		}
		content.WriteString(`</div>`)
	}

	selfPath := s.graphPath + suffix
	selfURL := selfPath + view.Query
	data := PageData{
		Title: "Graph - " + centerNode.Title,
		Breadcrumbs: []navigation.NavNode{
			{Title: "Home", FullPath: s.basePath + "/"},
			{Title: centerNode.Title, FullPath: centerNode.FullPath},
			{Title: "Graph", FullPath: selfURL},
		},
		ContentHTML:       template.HTML(content.String()),
		Nav:               view.Nav,
		CurrentPath:       centerNode.FullPath,
		IsHTMX:            isHTMX(r),
		LightCSS:          template.CSS(s.renderer.LightCSS()),
		DarkCSS:           template.CSS(s.renderer.DarkCSS()),
		Site:              s.siteCfg,
		BasePath:          s.basePath,
		TailwindURL:       s.assetURL("tailwind.css"),
		AppCSSURL:         s.assetURL("app.css"),
		AppJSURL:          s.assetURL("app.js"),
		HTMXURL:           s.assetURL("htmx.min.js"),
		MermaidURL:        s.assetURL("mermaid.min.js"),
		FaviconURL:        s.faviconURL(),
		SearchURL:         s.searchPath + view.Query,
		TasksURL:          s.tasksPath,
		GraphURL:          s.graphPath,
		BuildVersion:      s.version,
		BuildCommit:       s.commit,
		MetadataFacets:    view.Facets,
		FilterQuery:       view.Query,
		FiltersActive:     view.Active,
		MetadataFilterURL: selfPath,
	}
	if isHTMX(r) {
		w.Header().Set("HX-Push-Url", selfURL)
		if err := s.render(w, "page-fragment", data); err != nil {
			slog.Error("template error", "template", "page-fragment", "err", err)
		}
		return
	}
	if err := s.render(w, "base.html", data); err != nil {
		slog.Error("template error", "template", "base.html", "err", err)
	}
}

// graphNeighboursHandler returns an HTML fragment listing neighbours of a node.
// Used by HTMX [+] expand in ego-graph.
func (s *Server) graphNeighboursHandler(w http.ResponseWriter, r *http.Request) {
	view := s.metadataView(r)
	pagePath := r.URL.Query().Get("path")
	exclude := r.URL.Query().Get("exclude")

	// Collect neighbours: incoming + outgoing
	type nb struct{ url, title string }
	seen := map[string]bool{pagePath: true}
	if exclude != "" {
		seen[exclude] = true
	}
	var neighbours []nb

	for _, bl := range s.backlinks[pagePath] {
		if !seen[bl.FullPath] {
			seen[bl.FullPath] = true
			neighbours = append(neighbours, nb{bl.FullPath, bl.Title})
		}
	}
	for target, sources := range s.backlinks {
		for _, src := range sources {
			if src.FullPath == pagePath && !seen[target] {
				seen[target] = true
				title := path.Base(target)
				navigation.WalkNodes(s.getNav(), func(n *navigation.NavNode) {
					if n.FullPath == target {
						title = n.Title
					}
				})
				neighbours = append(neighbours, nb{target, title})
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if len(neighbours) == 0 {
		fmt.Fprint(w, `<span class="ego-nb-empty">no other neighbours</span>`)
		return
	}
	idx := s.getIdx()
	var b strings.Builder
	b.WriteString(`<table class="ego-nb-table"><thead><tr><th class="ego-nb-th ego-nb-th-num">#</th><th class="ego-nb-th">Nadpis</th><th class="ego-nb-th">Podkapitoly</th></tr></thead><tbody>`)
	for i, n := range neighbours {
		pageURL := n.url + view.Query
		var headingsStr string
		if idx != nil {
			if hh := idx.Headings(n.url); len(hh) > 0 {
				headingsStr = strings.Join(hh, ", ")
			}
		}
		b.WriteString(`<tr class="ego-nb-tr">`)
		b.WriteString(`<td class="ego-nb-td ego-nb-td-num">` + fmt.Sprintf("%d", i+1) + `</td>`)
		b.WriteString(`<td class="ego-nb-td"><a class="ego-nb-child" href="` + html.EscapeString(pageURL) + `" hx-get="` + html.EscapeString(pageURL) + `" hx-target="#page-content" hx-push-url="true">` + html.EscapeString(n.title) + `</a></td>`)
		b.WriteString(`<td class="ego-nb-td ego-nb-td-section">` + html.EscapeString(headingsStr) + `</td>`)
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table>`)
	fmt.Fprint(w, b.String())
}

func appendFilterQuery(rawURL, filterQuery string) string {
	if filterQuery == "" {
		return rawURL
	}
	separator := "?"
	if strings.Contains(rawURL, "?") {
		separator = "&"
	}
	return rawURL + separator + strings.TrimPrefix(filterQuery, "?")
}

func egoNodeID(urlPath string) string {
	s := strings.Trim(urlPath, "/")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "-", "_")
	if s == "" {
		return "root"
	}
	return s
}

// buildDepsTable builds an HTML dependency table for all pages.
func buildDepsTable(root *navigation.NavNode, bl navigation.BacklinkIndex, graphPath string) string {
	// Collect all pages and build outgoing index
	type row struct {
		node     *navigation.NavNode
		incoming []navigation.Backlink
		outgoing []navigation.Backlink
	}

	// Build outgoing map: page → pages it links to
	outgoingMap := map[string][]navigation.Backlink{}
	for target, sources := range bl {
		for _, src := range sources {
			outgoingMap[src.FullPath] = append(outgoingMap[src.FullPath], navigation.Backlink{
				Title: "", FullPath: target,
			})
		}
	}
	// Fill in titles for outgoing targets
	titleMap := map[string]string{}
	navigation.WalkNodes(root, func(n *navigation.NavNode) {
		if n.FSPath != "" {
			titleMap[n.FullPath] = n.Title
		}
	})
	for src, targets := range outgoingMap {
		for i, t := range targets {
			if title, ok := titleMap[t.FullPath]; ok {
				targets[i].Title = title
			}
		}
		outgoingMap[src] = targets
	}

	var rows []row
	navigation.WalkNodes(root, func(n *navigation.NavNode) {
		if n.FSPath == "" || n.IsDir {
			return
		}
		inc := bl[n.FullPath]
		out := outgoingMap[n.FullPath]
		if len(inc) == 0 && len(out) == 0 {
			return // skip isolated pages
		}
		rows = append(rows, row{n, inc, out})
	})

	if len(rows) == 0 {
		return `<div class="tasks-empty">No dependencies were found.</div>`
	}

	var b strings.Builder
	b.WriteString(`<div class="deps-table-wrap">`)
	b.WriteString(`<p class="deps-summary">Pages with at least one link. Click a page name to open its ego graph.</p>`)
	b.WriteString(`<table class="deps-table">`)
	b.WriteString(`<thead><tr><th>Page</th><th>Links to</th><th>Linked from</th></tr></thead>`)
	b.WriteString(`<tbody>`)
	for _, row := range rows {
		egoURL := graphPath + strings.TrimRight(row.node.FullPath, "/")[len(strings.TrimRight(graphPath, "/"))-len(graphPath):]
		// ego URL: graphPath + page path relative to basePath
		// simpler: graphPath + "/" + slug-path from node
		egoURL = graphPath + row.node.FullPath[strings.Index(row.node.FullPath[1:], "/")+1:]
		b.WriteString(`<tr>`)
		b.WriteString(`<td><a class="deps-page-link" href="` + html.EscapeString(egoURL) + `" hx-get="` + html.EscapeString(egoURL) + `" hx-target="#page-content" hx-push-url="true">`)
		b.WriteString(html.EscapeString(row.node.Title))
		b.WriteString(`</a></td>`)
		// Outgoing
		b.WriteString(`<td>`)
		for i, t := range row.outgoing {
			if i > 0 {
				b.WriteString(`, `)
			}
			title := t.Title
			if title == "" {
				title = path.Base(t.FullPath)
			}
			b.WriteString(`<a class="deps-link" href="` + html.EscapeString(t.FullPath) + `" hx-get="` + html.EscapeString(t.FullPath) + `" hx-target="#page-content" hx-push-url="true">` + html.EscapeString(title) + `</a>`)
		}
		b.WriteString(`</td>`)
		// Incoming
		b.WriteString(`<td>`)
		for i, t := range row.incoming {
			if i > 0 {
				b.WriteString(`, `)
			}
			b.WriteString(`<a class="deps-link" href="` + html.EscapeString(t.FullPath) + `" hx-get="` + html.EscapeString(t.FullPath) + `" hx-target="#page-content" hx-push-url="true">` + html.EscapeString(t.Title) + `</a>`)
		}
		b.WriteString(`</td>`)
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table></div>`)
	return b.String()
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
	start := time.Now()
	observeProxy := func(statusClass string) {
		s.metrics.apiProxyRequests.WithLabelValues(statusClass).Inc()
		s.metrics.apiProxyDuration.WithLabelValues(statusClass).Observe(time.Since(start).Seconds())
	}

	var req apiProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		observeProxy("4xx")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.URL == "" || req.Method == "" {
		observeProxy("4xx")
		http.Error(w, "method and url required", http.StatusBadRequest)
		return
	}
	if err := s.validateProxyTarget(r.Context(), req.URL); err != nil {
		observeProxy("4xx")
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
		observeProxy("4xx")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<div class="api-error">Invalid URL: %s</div>`, htmlEscapeStr(err.Error()))
		return
	}
	for k, v := range req.Headers {
		outReq.Header.Set(k, v)
	}

	resp, err := apiHTTPClient.Do(outReq)
	elapsed := time.Since(start)
	if err != nil {
		observeProxy("5xx")
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
	observeProxy(strings.TrimPrefix(statusClass, "api-status-"))

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
		attr, rawPath := subs[1], subs[2]
		// Don't double-prefix
		if strings.HasPrefix(rawPath, basePath+"/") || rawPath == basePath {
			return match
		}
		return attr + `="` + basePath + rawPath + `"`
	})
}

// mdLinkRe matches Markdown document links and preserves query strings/fragments.
var mdLinkRe = regexp.MustCompile(`href="([^"?#]*\.md)([?#][^"]*)?"`)

// rewriteMDLinks converts filesystem-relative Markdown links to navigation URLs.
func (s *Server) rewriteMDLinks(htmlStr, sourceFSPath string) string {
	return mdLinkRe.ReplaceAllStringFunc(htmlStr, func(match string) string {
		subs := mdLinkRe.FindStringSubmatch(match)
		href, suffix := subs[1], subs[2]

		// URL-decode to get the real path (Goldmark percent-encodes non-ASCII)
		decoded, err := url.PathUnescape(href)
		if err != nil {
			decoded = href
		}
		var targetPath string
		if strings.HasPrefix(decoded, "/") {
			targetPath = filepath.Join(s.contentDir, filepath.FromSlash(strings.TrimPrefix(decoded, "/")))
		} else {
			targetPath = filepath.Join(filepath.Dir(sourceFSPath), filepath.FromSlash(decoded))
		}
		targetPath = filepath.Clean(targetPath)

		rel, err := filepath.Rel(s.contentDir, targetPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return match
		}

		if target := navigation.FindNodeByFSPath(s.getNav(), targetPath); target != nil {
			return `href="` + target.FullPath + suffix + `"`
		}

		// Some generated repositories link Directory/Directory.md although the
		// directory itself is the document and the named file does not exist.
		targetDir := filepath.Dir(targetPath)
		fileSlug := navigation.SlugFromName(strings.TrimSuffix(filepath.Base(targetPath), filepath.Ext(targetPath)))
		if info, statErr := os.Stat(targetDir); statErr == nil && info.IsDir() &&
			fileSlug == navigation.SlugFromName(filepath.Base(targetDir)) {
			dirRel, relErr := filepath.Rel(s.contentDir, targetDir)
			if relErr == nil {
				parts := strings.Split(filepath.ToSlash(dirRel), "/")
				for i := range parts {
					parts[i] = navigation.SlugFromName(parts[i])
				}
				pageURL := s.basePath + "/" + strings.Join(parts, "/") + "/"
				if navigation.FindNode(s.getNav(), pageURL) != nil {
					return `href="` + pageURL + suffix + `"`
				}
			}
		}

		return match
	})
}
