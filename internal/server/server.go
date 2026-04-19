package server

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

// Server holds all dependencies and serves the documentation site.
type Server struct {
	mu         sync.RWMutex
	siteRoot   string
	basePath   string // e.g. "/docs"
	searchPath string // e.g. "/docs/search" or "/search"
	libraryURL string // non-empty when running as a book inside a LibraryServer
	siteCfg   *config.SiteConfig
	version   string
	commit    string
	nav       *navigation.NavNode
	idx       *search.Index
	renderer  *markdown.Renderer
	buildTime time.Time
	tmpl      *template.Template // nil in dev mode (re-parsed per request)
	tmplFS    fs.FS              // used in dev mode for live reloading
	devMode   bool
	mux       *http.ServeMux
	pageCache map[string]pageCacheEntry
	assetMeta map[string]assetMetadata
	metrics   *metrics
}

type pageCacheEntry struct {
	modTime time.Time
	result  *markdown.Result
}

// Reload swaps the navigation tree and search index atomically (used by --watch).
func (s *Server) Reload(nav *navigation.NavNode, idx *search.Index) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nav = nav
	s.idx = idx
	s.pageCache = make(map[string]pageCacheEntry)
	s.metrics.watchReloads.Inc()
}

func (s *Server) getNav() *navigation.NavNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nav
}

func (s *Server) getIdx() *search.Index {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.idx
}

// Config holds server configuration.
type Config struct {
	SiteRoot   string
	Port       string
	DevMode    bool
	SiteCfg    *config.SiteConfig
	Version    string
	Commit     string
	BuildTime  time.Time
	LibraryURL string // set by LibraryServer: URL back to the dashboard
}

func newFuncMap() template.FuncMap {
	return template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"add":      func(a, b int) int { return a + b },
		"fmtNum": func(n int) string {
			// Format integer with thousands separator: 12400 → "12,400"
			s := fmt.Sprintf("%d", n)
			if len(s) <= 3 {
				return s
			}
			var out []byte
			for i, c := range s {
				if i > 0 && (len(s)-i)%3 == 0 {
					out = append(out, ',')
				}
				out = append(out, byte(c))
			}
			return string(out)
		},
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict requires even number of arguments")
			}
			m := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				m[key] = values[i+1]
			}
			return m, nil
		},
	}
}

// New creates a Server from the given filesystem (embedded or OS), and registers routes.
// In dev mode (cfg.DevMode=true), templates are re-parsed on every request so
// changes to templates/ and assets/ are reflected immediately without rebuilding.
func New(cfg Config, staticFS fs.FS, nav *navigation.NavNode, idx *search.Index, renderer *markdown.Renderer) (*Server, error) {
	tmplFS, err := fs.Sub(staticFS, "templates")
	if err != nil {
		return nil, err
	}

	s := &Server{
		siteRoot:   cfg.SiteRoot,
		basePath:   cfg.SiteCfg.GetBasePath(),
		libraryURL: cfg.LibraryURL,
		siteCfg:   cfg.SiteCfg,
		version:   cfg.Version,
		commit:    cfg.Commit,
		nav:       nav,
		idx:       idx,
		renderer:  renderer,
		buildTime: cfg.BuildTime,
		tmplFS:    tmplFS,
		devMode:   cfg.DevMode,
		mux:       http.NewServeMux(),
		pageCache: make(map[string]pageCacheEntry),
	}
	s.metrics = newMetrics()

	if !cfg.DevMode {
		// Production: parse once at startup
		tmpl, err := parseTemplates(tmplFS)
		if err != nil {
			return nil, err
		}
		s.tmpl = tmpl
	}

	assetsFS, err := fs.Sub(staticFS, "assets")
	if err != nil {
		return nil, err
	}
	if !cfg.DevMode {
		s.assetMeta, err = buildAssetManifest(assetsFS)
		if err != nil {
			return nil, err
		}
	}
	s.mux.Handle("GET /assets/", s.assetHandler(assetsFS))

	basePath := cfg.SiteCfg.GetBasePath() // e.g. "/docs" or ""
	searchPath := "/search"
	if basePath != "" {
		searchPath = basePath + "/search"
	}
	s.searchPath = searchPath

	if basePath == "" {
		s.mux.HandleFunc("GET /{path...}", s.pageHandler)
	} else {
		s.mux.HandleFunc("GET /", s.redirectHandler)
		s.mux.HandleFunc("GET "+basePath+"/{path...}", s.pageHandler)
	}
	s.mux.HandleFunc("GET "+searchPath, s.searchHandler)
	s.mux.HandleFunc("POST /api-proxy", s.apiProxyHandler)
	s.mux.HandleFunc("GET /healthz", s.healthHandler)
	s.mux.HandleFunc("GET /livez", s.livenessHandler)
	s.mux.HandleFunc("GET /readyz", s.readinessHandler)
	s.mux.Handle("GET /metrics", s.metrics.handler())

	return s, nil
}

func (s *Server) getRenderedPage(fsPath string) (*markdown.Result, error) {
	info, err := os.Stat(fsPath)
	if err != nil {
		return nil, err
	}

	modTime := info.ModTime()

	s.mu.RLock()
	cached, ok := s.pageCache[fsPath]
	s.mu.RUnlock()
	if ok && cached.modTime.Equal(modTime) {
		s.metrics.pageRenderCache.WithLabelValues("hit").Inc()
		resultCopy := *cached.result
		return &resultCopy, nil
	}
	s.metrics.pageRenderCache.WithLabelValues("miss").Inc()

	src, err := os.ReadFile(fsPath)
	if err != nil {
		return nil, err
	}
	res, err := s.renderer.Render(src)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.pageCache[fsPath] = pageCacheEntry{
		modTime: modTime,
		result:  res,
	}
	s.mu.Unlock()

	resultCopy := *res
	return &resultCopy, nil
}

func parseTemplates(tmplFS fs.FS) (*template.Template, error) {
	return template.New("").Funcs(newFuncMap()).ParseFS(tmplFS, "*.html")
}

// getTemplate returns the template set — either cached (prod) or freshly parsed (dev).
func (s *Server) getTemplate() (*template.Template, error) {
	if !s.devMode {
		return s.tmpl, nil
	}
	return parseTemplates(s.tmplFS)
}

// Handler returns the HTTP handler with middleware applied.
func (s *Server) Handler() http.Handler {
	cache := cacheMiddleware
	if s.devMode {
		cache = devCacheMiddleware
	}
	return recoveryMiddleware(loggingMiddleware(metricsMiddleware(s.metrics, gzipMiddleware(cache(s.mux)))))
}

func (s *Server) redirectHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, s.basePath+"/", http.StatusMovedPermanently)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	s.readinessHandler(w, r)
}

func (s *Server) livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	ready := s.getNav() != nil && s.getIdx() != nil && s.renderer != nil
	if !ready || (!s.devMode && s.tmpl == nil) {
		http.Error(w, "NOT READY", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
