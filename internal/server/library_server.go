package server

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mares/dyno/internal/library"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/search"
)

// LibraryServer serves a collection of books (multiple --site flags).
type LibraryServer struct {
	title    string
	logoText string
	subtitle string
	basePath string
	books    []*library.Book
	servers  map[string]*Server // slug → per-book Server
	tmplFS   fs.FS
	devMode  bool
	tmpl     *template.Template
	mux      *http.ServeMux
	metrics  *metrics
}

// LibraryConfig holds top-level configuration for library mode.
type LibraryConfig struct {
	Title     string // used in <title> (e.g. "Dyno Docs")
	LogoText  string // short name in navbar (e.g. "Dyno")
	Subtitle  string
	BasePath  string
	DevMode   bool
	Version   string
	Commit    string
	BuildTime time.Time
}

// NewLibrary creates a LibraryServer that routes across all books.
func NewLibrary(cfg LibraryConfig, staticFS fs.FS, books []*library.Book, renderer *markdown.Renderer) (*LibraryServer, error) {
	tmplFS, err := fs.Sub(staticFS, "templates")
	if err != nil {
		return nil, err
	}

	assetsFS, err := fs.Sub(staticFS, "assets")
	if err != nil {
		return nil, err
	}

	ls := &LibraryServer{
		title:    cfg.Title,
		logoText: cfg.LogoText,
		subtitle: cfg.Subtitle,
		basePath: cfg.BasePath,
		books:    books,
		servers:  make(map[string]*Server),
		tmplFS:   tmplFS,
		devMode:  cfg.DevMode,
		mux:      http.NewServeMux(),
		metrics:  newMetrics(),
	}

	if !cfg.DevMode {
		tmpl, err := parseTemplates(tmplFS)
		if err != nil {
			return nil, err
		}
		ls.tmpl = tmpl
	}

	// Assets served at /assets/
	ls.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assetsFS))))

	// Dashboard
	base := cfg.BasePath
	searchPath := "/_search"
	if base != "" {
		searchPath = base + "/_search"
	}
	if base == "" {
		ls.mux.HandleFunc("GET /{$}", ls.dashboardHandler)
	} else {
		ls.mux.HandleFunc("GET /", ls.rootRedirectHandler)
		ls.mux.HandleFunc("GET "+base+"/{$}", ls.dashboardHandler)
		ls.mux.HandleFunc("GET "+base, ls.dashboardHandler)
	}

	ls.mux.HandleFunc("GET /_search", ls.searchHandler)
	if searchPath != "/_search" {
		ls.mux.HandleFunc("GET "+searchPath, ls.searchHandler)
	}
	ls.mux.HandleFunc("GET /healthz", ls.healthHandler)
	ls.mux.HandleFunc("GET /livez", ls.healthHandler)
	ls.mux.HandleFunc("GET /readyz", ls.healthHandler)
	ls.mux.Handle("GET /metrics", ls.metrics.handler())
	ls.mux.HandleFunc("POST /api-proxy", ls.apiProxyHandlerShared())

	// Per-book sub-servers: register all their routes under basePath/slug/
	for _, book := range books {
		bookBasePath := strings.TrimRight(base, "/") + "/" + book.Slug
		dashboardURL := base + "/"
		if base == "" {
			dashboardURL = "/"
		}
		bookCfg := Config{
			SiteRoot:   book.SiteRoot,
			ContentDir: book.ContentDir,
			Port:       "",
			DevMode:    cfg.DevMode,
			SiteCfg:    book.Cfg,
			Version:    cfg.Version,
			Commit:     cfg.Commit,
			BuildTime:  cfg.BuildTime,
			LibraryURL: dashboardURL,
		}
		// Override the book's base_path to bookBasePath so nav links are correct.
		bookCfg.SiteCfg = cloneCfgWithBasePath(book.Cfg, bookBasePath)

		srv, err := newBookServer(bookCfg, staticFS, book, renderer)
		if err != nil {
			return nil, fmt.Errorf("book %q: %w", book.Slug, err)
		}
		ls.servers[book.Slug] = srv

		// Route all requests under basePath/slug/ to this book's mux.
		pattern := "GET " + bookBasePath + "/{path...}"
		slug := book.Slug
		ls.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			ls.servers[slug].mux.ServeHTTP(w, r)
		})
		slog.Info("registered book", "slug", book.Slug, "path", bookBasePath, "pages", book.PageCount)
	}

	return ls, nil
}

// ReloadBook swaps the nav/search index for a single book atomically (used by git auto-pull).
func (ls *LibraryServer) ReloadBook(book *library.Book) {
	srv, ok := ls.servers[book.Slug]
	if !ok {
		slog.Warn("git reload: unknown book slug", "slug", book.Slug)
		return
	}
	srv.Reload(book.Nav, book.Idx)
}

// Handler returns the HTTP handler with middleware applied.
func (ls *LibraryServer) Handler() http.Handler {
	cache := cacheMiddleware
	if ls.devMode {
		cache = devCacheMiddleware
	}
	return recoveryMiddleware(loggingMiddleware(metricsMiddleware(ls.metrics, gzipMiddleware(cache(ls.mux)))))
}

func (ls *LibraryServer) rootRedirectHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, ls.basePath+"/", http.StatusMovedPermanently)
}

func (ls *LibraryServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (ls *LibraryServer) getTemplate() (*template.Template, error) {
	if !ls.devMode {
		return ls.tmpl, nil
	}
	return parseTemplates(ls.tmplFS)
}

// bookCardData is passed to the library.html template for each book card.
type bookCardData struct {
	Title       string
	Description string
	Icon        string
	AccentColor string
	BookPath    string
	PageCount   int
	WordCount   int
	TopChapters []chapterEntry
}

type chapterEntry struct {
	Title string
}

func (ls *LibraryServer) dashboardHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := ls.getTemplate()
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	cards := make([]bookCardData, 0, len(ls.books))
	for _, book := range ls.books {
		color := book.Cfg.Color
		if color == "" {
			color = "#0ea5e9"
		}
		bookPath := strings.TrimRight(ls.basePath, "/") + "/" + book.Slug
		var chapters []chapterEntry
		for _, ch := range book.TopLevel(5) {
			chapters = append(chapters, chapterEntry{Title: ch.Title})
		}
		cards = append(cards, bookCardData{
			Title:       book.Cfg.Title,
			Description: book.Cfg.Description,
			Icon:        book.Cfg.Icon,
			AccentColor: color,
			BookPath:    bookPath,
			PageCount:   book.PageCount,
			WordCount:   book.WordCount,
			TopChapters: chapters,
		})
	}

	// Reuse asset URLs from any book server.
	appJSURL, appCSSURL, tailwindURL, htmxURL, mermaidURL := "/assets/app.js", "/assets/app.css", "/assets/tailwind.css", "/assets/htmx.min.js", "/assets/mermaid.min.js"
	for _, srv := range ls.servers {
		appJSURL = srv.assetURL("app.js")
		appCSSURL = srv.assetURL("app.css")
		tailwindURL = srv.assetURL("tailwind.css")
		htmxURL = srv.assetURL("htmx.min.js")
		mermaidURL = srv.assetURL("mermaid.min.js")
		break
	}

	data := LibraryData{
		Title:       ls.title,
		LogoText:    ls.logoText,
		Subtitle:    ls.subtitle,
		Books:       cards,
		BasePath:    ls.basePath,
		SearchURL:   strings.TrimRight(ls.basePath, "/") + "/search",
		AppJSURL:    appJSURL,
		AppCSSURL:   appCSSURL,
		TailwindURL: tailwindURL,
		HTMXURL:     htmxURL,
		MermaidURL:  mermaidURL,
		IsHTMX:      r.Header.Get("HX-Request") == "true",
	}

	if err := tmpl.ExecuteTemplate(w, "library-base.html", data); err != nil {
		slog.Error("library template error", "err", err)
	}
}

func (ls *LibraryServer) searchHandler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var results []search.SearchResult
	if q != "" {
		for _, book := range ls.books {
			for _, res := range book.Idx.Search(q) {
				res.BookTitle = book.Cfg.Title
				res.BookSlug = book.Slug
				results = append(results, res)
			}
		}
		// Re-sort merged results by score descending
		sortResults(results)
		if len(results) > 20 {
			results = results[:20]
		}
	}

	// Build asset URLs
	appJSURL := "/assets/app.js"
	appCSSURL := "/assets/app.css"
	tailwindURL := "/assets/tailwind.css"
	for _, srv := range ls.servers {
		appJSURL = srv.assetURL("app.js")
		appCSSURL = srv.assetURL("app.css")
		tailwindURL = srv.assetURL("tailwind.css")
		break
	}

	data := SearchData{
		Query:       q,
		Results:     results,
		IsHTMX:      r.Header.Get("HX-Request") == "true",
		Title:       "Search",
		BasePath:    ls.basePath,
		SearchURL:   strings.TrimRight(ls.basePath, "/") + "/search",
		AppJSURL:    appJSURL,
		AppCSSURL:   appCSSURL,
		TailwindURL: tailwindURL,
	}

	tmpl, err := ls.getTemplate()
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	if data.IsHTMX {
		if err := tmpl.ExecuteTemplate(w, "search-results", data); err != nil {
			slog.Error("search template error", "err", err)
		}
		return
	}
	if err := tmpl.ExecuteTemplate(w, "search-page.html", data); err != nil {
		slog.Error("search template error", "err", err)
	}
}

func sortResults(results []search.SearchResult) {
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Score > results[j-1].Score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
}

// apiProxyHandlerShared returns a handler that delegates to the first book server's proxy.
func (ls *LibraryServer) apiProxyHandlerShared() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, srv := range ls.servers {
			srv.apiProxyHandler(w, r)
			return
		}
		http.Error(w, "no books configured", http.StatusServiceUnavailable)
	}
}
