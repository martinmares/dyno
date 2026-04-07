package server

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"sync"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

// Server holds all dependencies and serves the documentation site.
type Server struct {
	mu        sync.RWMutex
	siteRoot  string
	basePath  string // e.g. "/docs"
	siteCfg   *config.SiteConfig
	nav       *navigation.NavNode
	idx       *search.Index
	renderer  *markdown.Renderer
	tmpl      *template.Template // nil in dev mode (re-parsed per request)
	tmplFS    fs.FS              // used in dev mode for live reloading
	devMode   bool
	mux       *http.ServeMux
}

// Reload swaps the navigation tree and search index atomically (used by --watch).
func (s *Server) Reload(nav *navigation.NavNode, idx *search.Index) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nav = nav
	s.idx = idx
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
	SiteRoot string
	Port     string
	DevMode  bool
	SiteCfg  *config.SiteConfig
}

func newFuncMap() template.FuncMap {
	return template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"add":      func(a, b int) int { return a + b },
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
		siteRoot: cfg.SiteRoot,
		basePath: cfg.SiteCfg.GetBasePath(),
		siteCfg:  cfg.SiteCfg,
		nav:      nav,
		idx:      idx,
		renderer: renderer,
		tmplFS:   tmplFS,
		devMode:  cfg.DevMode,
		mux:      http.NewServeMux(),
	}

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
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assetsFS))))

	basePath := cfg.SiteCfg.GetBasePath() // e.g. "/docs" or ""
	if basePath == "" {
		// Root: pages served directly at /{path...}, no redirect needed
		s.mux.HandleFunc("GET /{path...}", s.pageHandler)
	} else {
		s.mux.HandleFunc("GET /", s.redirectHandler)
		s.mux.HandleFunc("GET "+basePath+"/{path...}", s.pageHandler)
	}
	s.mux.HandleFunc("GET /search", s.searchHandler)
	s.mux.HandleFunc("GET /healthz", s.healthHandler)

	return s, nil
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
	return recoveryMiddleware(loggingMiddleware(gzipMiddleware(cacheMiddleware(s.mux))))
}

func (s *Server) redirectHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, s.basePath+"/", http.StatusMovedPermanently)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
