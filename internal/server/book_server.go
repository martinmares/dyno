package server

import (
	"io/fs"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/library"
	"github.com/mares/dyno/internal/markdown"
)

// newBookServer creates a per-book Server (without middleware) for use inside LibraryServer.
// It reuses the full Server logic but registers routes under bookCfg.SiteCfg.GetBasePath().
func newBookServer(cfg Config, staticFS fs.FS, book *library.Book, renderer *markdown.Renderer) (*Server, error) {
	return New(cfg, staticFS, book.Nav, book.Idx, renderer)
}

// cloneCfgWithBasePath returns a shallow copy of cfg with BasePath overridden.
func cloneCfgWithBasePath(src *config.SiteConfig, basePath string) *config.SiteConfig {
	copy := *src
	copy.BasePath = &basePath
	return &copy
}
