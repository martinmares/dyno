package mcpengine

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/library"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
	"github.com/mares/dyno/internal/sitepath"
	"github.com/mares/dyno/internal/tasks"
)

type Mode string

const (
	ModeSingle  Mode = "single"
	ModeLibrary Mode = "library"
)

const DefaultBookSlug = "_default"

type Engine struct {
	mode          Mode
	publicBaseURL string
	basePath      string
	renderer      *markdown.Renderer
	books         []*Book
	booksBySlug   map[string]*Book
}

type Book struct {
	Slug       string
	Title      string
	SiteRoot   string
	ContentDir string
	BasePath   string
	Cfg        *config.SiteConfig
	Nav        *navigation.NavNode
	Idx        *search.Index
	Tasks      *tasks.Index
}

type SearchResult struct {
	BookSlug  string  `json:"book_slug"`
	BookTitle string  `json:"book_title"`
	Title     string  `json:"title"`
	Path      string  `json:"path"`
	URL       string  `json:"url"`
	Snippet   string  `json:"snippet"`
	Score     float64 `json:"score"`
}

type PageSummary struct {
	BookSlug string `json:"book_slug"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	URL      string `json:"url"`
	IsDir    bool   `json:"is_dir"`
	Depth    int    `json:"depth"`
}

type Page struct {
	BookSlug    string               `json:"book_slug"`
	BookTitle   string               `json:"book_title"`
	Title       string               `json:"title"`
	Path        string               `json:"path"`
	URL         string               `json:"url"`
	Markdown    string               `json:"markdown"`
	PlainText   string               `json:"plain_text"`
	HTML        string               `json:"html"`
	Headings    []markdown.TOCEntry  `json:"headings"`
	Frontmatter markdown.Frontmatter `json:"frontmatter"`
}

type PageSection struct {
	BookSlug  string              `json:"book_slug"`
	BookTitle string              `json:"book_title"`
	PageTitle string              `json:"page_title"`
	PagePath  string              `json:"page_path"`
	PageURL   string              `json:"page_url"`
	Heading   string              `json:"heading"`
	Level     int                 `json:"level"`
	Markdown  string              `json:"markdown"`
	PlainText string              `json:"plain_text"`
	HTML      string              `json:"html"`
	Headings  []markdown.TOCEntry `json:"headings"`
}

type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type ResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

func Load(siteDirs []string, publicBaseURL string) (*Engine, error) {
	if len(siteDirs) == 0 {
		return nil, fmt.Errorf("at least one --site is required")
	}

	absSites := make([]string, 0, len(siteDirs))
	for _, siteDir := range siteDirs {
		paths, err := sitepath.Resolve(siteDir)
		if err != nil {
			return nil, fmt.Errorf("invalid site path %q: %w", siteDir, err)
		}
		absSites = append(absSites, paths.ContentDir)
	}

	firstPaths, err := sitepath.Resolve(absSites[0])
	if err != nil {
		return nil, err
	}
	firstRoot := firstPaths.RootDir
	templateEnv, err := config.LoadTemplateEnv(firstRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to load template env: %w", err)
	}
	renderer, err := markdown.NewRendererWithEnv(templateEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create renderer: %w", err)
	}

	engine := &Engine{
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
		renderer:      renderer,
		booksBySlug:   make(map[string]*Book),
	}

	if len(absSites) == 1 {
		book, err := loadSingleBook(absSites[0], renderer)
		if err != nil {
			return nil, err
		}
		engine.mode = ModeSingle
		engine.basePath = book.BasePath
		engine.books = []*Book{book}
		engine.booksBySlug[book.Slug] = book
		return engine, nil
	}

	globalCfg, err := config.LoadForContent(firstRoot, absSites[0])
	if err != nil {
		return nil, fmt.Errorf("failed to load global config: %w", err)
	}
	globalBasePath := globalCfg.GetBasePath()

	plainText := func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	}

	engine.mode = ModeLibrary
	engine.basePath = globalBasePath

	for _, siteDir := range absSites {
		libBook, err := library.Load(siteDir, globalBasePath, plainText, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to load book %q: %w", siteDir, err)
		}
		book := &Book{
			Slug:       libBook.Slug,
			Title:      libBook.Cfg.Title,
			SiteRoot:   libBook.SiteRoot,
			ContentDir: libBook.ContentDir,
			BasePath:   strings.TrimRight(globalBasePath, "/") + "/" + libBook.Slug,
			Cfg:        libBook.Cfg,
			Nav:        libBook.Nav,
			Idx:        libBook.Idx,
			Tasks:      nil,
		}
		taskIndex, err := tasks.BuildIndex(book.Slug, book.Title, book.ContentDir, book.Nav)
		if err != nil {
			return nil, fmt.Errorf("failed to build task index for %q: %w", siteDir, err)
		}
		book.Tasks = taskIndex
		if _, exists := engine.booksBySlug[book.Slug]; exists {
			return nil, fmt.Errorf("duplicate book slug %q", book.Slug)
		}
		engine.books = append(engine.books, book)
		engine.booksBySlug[book.Slug] = book
	}

	sort.Slice(engine.books, func(i, j int) bool {
		return engine.books[i].Slug < engine.books[j].Slug
	})

	return engine, nil
}

func loadSingleBook(siteDir string, renderer *markdown.Renderer) (*Book, error) {
	paths, err := sitepath.Resolve(siteDir)
	if err != nil {
		return nil, err
	}
	siteRoot := paths.RootDir
	contentDir := paths.ContentDir
	siteCfg, err := config.LoadForContent(siteRoot, contentDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load dyno.yaml: %w", err)
	}

	basePath := siteCfg.GetBasePath()
	nav, err := navigation.BuildTreeWithFilter(contentDir, basePath, navigation.Filter{
		Include: siteCfg.ContentInclude,
		Exclude: siteCfg.ContentExclude,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build navigation tree: %w", err)
	}

	idx, err := search.BuildIndex(nav, func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build search index: %w", err)
	}
	taskIndex, err := tasks.BuildIndex(DefaultBookSlug, siteCfg.Title, contentDir, nav)
	if err != nil {
		return nil, fmt.Errorf("failed to build task index: %w", err)
	}

	return &Book{
		Slug:       DefaultBookSlug,
		Title:      siteCfg.Title,
		SiteRoot:   siteRoot,
		ContentDir: contentDir,
		BasePath:   basePath,
		Cfg:        siteCfg,
		Nav:        nav,
		Idx:        idx,
		Tasks:      taskIndex,
	}, nil
}

func (e *Engine) Mode() Mode {
	return e.mode
}

func (e *Engine) BasePath() string {
	return e.basePath
}

func (e *Engine) ListBooks() []*Book {
	return append([]*Book(nil), e.books...)
}

func (e *Engine) ResolveBook(slug string) (*Book, error) {
	if e.mode == ModeSingle {
		if slug == "" || slug == DefaultBookSlug {
			return e.books[0], nil
		}
		return nil, fmt.Errorf("book_slug is not supported in single-site mode")
	}
	if slug == "" {
		return nil, fmt.Errorf("book_slug is required in library mode")
	}
	book := e.booksBySlug[slug]
	if book == nil {
		return nil, fmt.Errorf("unknown book_slug %q", slug)
	}
	return book, nil
}

func (e *Engine) Search(query, bookSlug string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	if bookSlug != "" {
		book, err := e.ResolveBook(bookSlug)
		if err != nil {
			return nil, err
		}
		return e.searchBook(book, query, limit), nil
	}

	results := make([]SearchResult, 0)
	for _, book := range e.books {
		results = append(results, e.searchBook(book, query, limit)...)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if results[i].BookSlug == results[j].BookSlug {
				return results[i].Path < results[j].Path
			}
			return results[i].BookSlug < results[j].BookSlug
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (e *Engine) searchBook(book *Book, query string, limit int) []SearchResult {
	raw := book.Idx.Search(query)
	results := make([]SearchResult, 0, len(raw))
	for _, hit := range raw {
		results = append(results, SearchResult{
			BookSlug:  book.Slug,
			BookTitle: book.Title,
			Title:     stripMarkTags(hit.Title),
			Path:      relativePath(book, hit.Path),
			URL:       e.publicURL(hit.Path),
			Snippet:   stripMarkTags(hit.Snippet),
			Score:     hit.Score,
		})
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func (e *Engine) ListPages(bookSlug, prefix string) ([]PageSummary, error) {
	book, err := e.ResolveBook(bookSlug)
	if err != nil {
		return nil, err
	}

	prefix = normalizeRelativePath(prefix)
	pages := navigation.FlatPages(book.Nav)
	out := make([]PageSummary, 0, len(pages))
	for _, node := range pages {
		path := relativePath(book, node.FullPath)
		if prefix != "" && !strings.HasPrefix(strings.TrimPrefix(path, "/"), strings.TrimPrefix(prefix, "/")) {
			continue
		}
		out = append(out, PageSummary{
			BookSlug: book.Slug,
			Title:    node.Title,
			Path:     path,
			URL:      e.publicURL(node.FullPath),
			IsDir:    node.IsDir,
			Depth:    node.Depth,
		})
	}
	return out, nil
}

func (e *Engine) GetNavigation(bookSlug string) ([]PageSummary, error) {
	book, err := e.ResolveBook(bookSlug)
	if err != nil {
		return nil, err
	}

	var out []PageSummary
	var walk func(*navigation.NavNode)
	walk = func(node *navigation.NavNode) {
		if node != book.Nav {
			out = append(out, PageSummary{
				BookSlug: book.Slug,
				Title:    node.Title,
				Path:     relativePath(book, node.FullPath),
				URL:      e.publicURL(node.FullPath),
				IsDir:    node.IsDir,
				Depth:    node.Depth,
			})
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(book.Nav)
	return out, nil
}

func (e *Engine) GetPage(bookSlug, path string) (*Page, error) {
	book, err := e.ResolveBook(bookSlug)
	if err != nil {
		return nil, err
	}

	fullPath := path
	if !strings.HasPrefix(fullPath, book.BasePath) {
		fullPath = joinURLPath(book.BasePath, path)
	}

	node := navigation.FindNode(book.Nav, fullPath)
	if node == nil {
		node = navigation.FindNode(book.Nav, strings.TrimRight(fullPath, "/"))
	}
	if node == nil {
		node = navigation.FindNode(book.Nav, strings.TrimRight(fullPath, "/")+"/")
	}
	if node == nil || node.FSPath == "" {
		return nil, fmt.Errorf("page not found: %s", path)
	}

	raw, err := os.ReadFile(node.FSPath)
	if err != nil {
		return nil, err
	}

	renderOpts := markdown.RenderOptions{}
	if book.Tasks != nil {
		renderOpts.TaskRenderer = book.Tasks.Render
	}
	rendered, err := e.renderer.RenderWithOptions(raw, renderOpts)
	if err != nil {
		return nil, err
	}
	plainText, err := e.renderer.ToPlainText(raw)
	if err != nil {
		return nil, err
	}

	title := rendered.Title
	if title == "" {
		title = node.Title
	}

	return &Page{
		BookSlug:    book.Slug,
		BookTitle:   book.Title,
		Title:       title,
		Path:        relativePath(book, node.FullPath),
		URL:         e.publicURL(node.FullPath),
		Markdown:    string(raw),
		PlainText:   plainText,
		HTML:        rendered.HTML,
		Headings:    rendered.TOC,
		Frontmatter: rendered.Frontmatter,
	}, nil
}

func (e *Engine) GetPageSection(bookSlug, path, heading string) (*PageSection, error) {
	book, err := e.ResolveBook(bookSlug)
	if err != nil {
		return nil, err
	}

	page, err := e.GetPage(bookSlug, path)
	if err != nil {
		return nil, err
	}

	blocks := parseMarkdownSections(page.Markdown)
	target := normalizeHeading(heading)
	for _, block := range blocks {
		if normalizeHeading(block.Heading) != target {
			continue
		}

		html, plain, toc, err := e.renderSection(book, block.Markdown)
		if err != nil {
			return nil, err
		}

		return &PageSection{
			BookSlug:  book.Slug,
			BookTitle: book.Title,
			PageTitle: page.Title,
			PagePath:  page.Path,
			PageURL:   page.URL,
			Heading:   block.Heading,
			Level:     block.Level,
			Markdown:  block.Markdown,
			PlainText: plain,
			HTML:      html,
			Headings:  toc,
		}, nil
	}

	return nil, fmt.Errorf("heading not found: %s", heading)
}

func (e *Engine) ListResources(cursor string, limit int) ([]Resource, string, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	all := make([]Resource, 0)
	for _, book := range e.books {
		all = append(all, Resource{
			URI:         resourceURIForBook(book.Slug),
			Name:        book.Title,
			Description: book.Cfg.Description,
			MimeType:    "application/json",
		})
		all = append(all, Resource{
			URI:         resourceURIForNavigation(book.Slug),
			Name:        book.Title + " navigation",
			Description: "Ordered navigation tree for the dyno book",
			MimeType:    "application/json",
		})

		pages, err := e.ListPages(book.Slug, "")
		if err != nil {
			return nil, "", err
		}
		for _, page := range pages {
			if page.IsDir {
				continue
			}
			all = append(all, Resource{
				URI:         resourceURIForPage(book.Slug, page.Path),
				Name:        page.Title,
				Description: page.URL,
				MimeType:    "text/markdown",
			})
		}
	}

	start := 0
	if cursor != "" {
		value, err := strconv.Atoi(cursor)
		if err != nil || value < 0 {
			return nil, "", fmt.Errorf("invalid cursor")
		}
		start = value
	}
	if start > len(all) {
		return nil, "", fmt.Errorf("cursor out of range")
	}

	end := start + limit
	if end > len(all) {
		end = len(all)
	}

	nextCursor := ""
	if end < len(all) {
		nextCursor = strconv.Itoa(end)
	}
	return all[start:end], nextCursor, nil
}

func (e *Engine) ReadResource(uri string) ([]ResourceContents, error) {
	switch {
	case strings.HasPrefix(uri, "dyno://book/") && strings.HasSuffix(uri, "/navigation"):
		bookSlug := strings.TrimSuffix(strings.TrimPrefix(uri, "dyno://book/"), "/navigation")
		items, err := e.GetNavigation(bookSlug)
		if err != nil {
			return nil, err
		}
		text, err := toJSON(items)
		if err != nil {
			return nil, err
		}
		return []ResourceContents{{
			URI:      uri,
			MimeType: "application/json",
			Text:     text,
		}}, nil
	case strings.HasPrefix(uri, "dyno://book/") && strings.Contains(uri, "/page?path="):
		bookSlug, relPath, err := parsePageResourceURI(uri)
		if err != nil {
			return nil, err
		}
		page, err := e.GetPage(bookSlug, relPath)
		if err != nil {
			return nil, err
		}
		return []ResourceContents{{
			URI:      uri,
			MimeType: "text/markdown",
			Text:     page.Markdown,
		}}, nil
	case strings.HasPrefix(uri, "dyno://book/"):
		bookSlug := strings.TrimPrefix(uri, "dyno://book/")
		bookSlug = strings.TrimSuffix(bookSlug, "/")
		book, err := e.ResolveBook(bookSlug)
		if err != nil {
			return nil, err
		}
		payload := map[string]any{
			"slug":        book.Slug,
			"title":       book.Title,
			"base_path":   book.BasePath,
			"description": book.Cfg.Description,
			"public_url":  e.publicURL(book.BasePath),
		}
		text, err := toJSON(payload)
		if err != nil {
			return nil, err
		}
		return []ResourceContents{{
			URI:      uri,
			MimeType: "application/json",
			Text:     text,
		}}, nil
	default:
		return nil, fmt.Errorf("unknown resource URI: %s", uri)
	}
}

func (e *Engine) publicURL(path string) string {
	if e.publicBaseURL == "" {
		return path
	}
	return e.publicBaseURL + path
}

func relativePath(book *Book, fullPath string) string {
	trimmedBookBase := strings.TrimRight(book.BasePath, "/")
	trimmedFull := strings.TrimRight(fullPath, "/")
	if trimmedFull == trimmedBookBase {
		return "/"
	}
	rel := strings.TrimPrefix(trimmedFull, trimmedBookBase)
	if rel == "" {
		return "/"
	}
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	if strings.HasSuffix(fullPath, "/") && rel != "/" {
		rel += "/"
	}
	return rel
}

func normalizeRelativePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func joinURLPath(base, path string) string {
	base = strings.TrimRight(base, "/")
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		if base == "" {
			return "/"
		}
		return base + "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func stripMarkTags(s string) string {
	s = strings.ReplaceAll(s, "<mark>", "")
	s = strings.ReplaceAll(s, "</mark>", "")
	return s
}

type sectionBlock struct {
	Heading  string
	Level    int
	Markdown string
}

func parseMarkdownSections(src string) []sectionBlock {
	lines := strings.Split(src, "\n")
	var sections []sectionBlock
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		level := headingLevel(line)
		if level == 0 {
			continue
		}
		heading := strings.TrimSpace(line[level+1:])
		start := i
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			nextLevel := headingLevel(lines[j])
			if nextLevel > 0 && nextLevel <= level {
				end = j
				break
			}
		}
		sections = append(sections, sectionBlock{
			Heading:  heading,
			Level:    level,
			Markdown: strings.TrimSpace(strings.Join(lines[start:end], "\n")),
		})
	}
	return sections
}

func headingLevel(line string) int {
	trimmed := strings.TrimLeft(line, " ")
	if !strings.HasPrefix(trimmed, "#") {
		return 0
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0
	}
	if len(trimmed) <= level || trimmed[level] != ' ' {
		return 0
	}
	return level
}

func normalizeHeading(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (e *Engine) renderSection(book *Book, src string) (html string, plain string, toc []markdown.TOCEntry, err error) {
	renderOpts := markdown.RenderOptions{}
	if book != nil && book.Tasks != nil {
		renderOpts.TaskRenderer = book.Tasks.Render
	}
	result, err := e.renderer.RenderWithOptions([]byte(src), renderOpts)
	if err != nil {
		return "", "", nil, err
	}
	plain, err = e.renderer.ToPlainText([]byte(src))
	if err != nil {
		return "", "", nil, err
	}
	return result.HTML, plain, result.TOC, nil
}

func resourceURIForBook(slug string) string {
	return "dyno://book/" + slug
}

func resourceURIForNavigation(slug string) string {
	return "dyno://book/" + slug + "/navigation"
}

func resourceURIForPage(slug, path string) string {
	return "dyno://book/" + slug + "/page?path=" + url.QueryEscape(path)
}

func parsePageResourceURI(uri string) (bookSlug, relPath string, err error) {
	rest := strings.TrimPrefix(uri, "dyno://book/")
	parts := strings.SplitN(rest, "/page?path=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid page resource URI")
	}
	bookSlug = strings.TrimSuffix(parts[0], "/")
	relPath, err = url.QueryUnescape(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("invalid page resource URI path: %w", err)
	}
	if relPath == "" {
		relPath = "/"
	}
	return bookSlug, relPath, nil
}

func toJSON(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
