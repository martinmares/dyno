package library

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

// Book represents a single documentation site within a library collection.
type Book struct {
	SiteRoot string
	Cfg      *config.SiteConfig
	Slug     string // URL segment, e.g. "monitoring"
	Nav      *navigation.NavNode
	Idx      *search.Index

	// Stats computed at load time
	PageCount int
	WordCount int
}

// TopLevel returns up to maxItems top-level navigation children for the dashboard card.
func (b *Book) TopLevel(maxItems int) []*navigation.NavNode {
	if b.Nav == nil {
		return nil
	}
	out := make([]*navigation.NavNode, 0, maxItems)
	for _, ch := range b.Nav.Children {
		if len(out) >= maxItems {
			break
		}
		out = append(out, ch)
	}
	return out
}

// Load reads config, navigation and search index for a single site directory.
// basePath is the global base_path (e.g. "/docs"); each book gets basePath+"/"+slug.
func Load(siteDir string, globalBasePath string, plainText func(string) (string, error)) (*Book, error) {
	// Determine siteRoot: if siteDir itself has a site/ subdir, that's the root.
	// Otherwise the parent is the root and siteDir is the content dir.
	siteRoot := siteDir
	if _, err := os.Stat(filepath.Join(siteDir, "site")); os.IsNotExist(err) {
		siteRoot = filepath.Dir(siteDir)
	}

	// Load book config: prefer dyno.yaml in siteDir itself, fall back to siteRoot.
	cfgDir := siteRoot
	if config.Exists(siteDir) {
		cfgDir = siteDir
	}
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return nil, err
	}

	slug := cfg.GetSlug(filepath.Base(siteDir))
	bookBasePath := strings.TrimRight(globalBasePath, "/") + "/" + slug

	nav, err := navigation.BuildTree(siteRoot, bookBasePath)
	if err != nil {
		return nil, err
	}

	idx, err := search.BuildIndex(nav, plainText)
	if err != nil {
		return nil, err
	}

	pages, words := collectStats(nav)

	return &Book{
		SiteRoot:  siteRoot,
		Cfg:       cfg,
		Slug:      slug,
		Nav:       nav,
		Idx:       idx,
		PageCount: pages,
		WordCount:  words,
	}, nil
}

func collectStats(node *navigation.NavNode) (pages, words int) {
	if node.FSPath != "" {
		pages++
		data, err := os.ReadFile(node.FSPath)
		if err == nil {
			words += countWords(string(data))
		}
	}
	for _, ch := range node.Children {
		p, w := collectStats(ch)
		pages += p
		words += w
	}
	return
}

func countWords(s string) int {
	return len(strings.Fields(s))
}
