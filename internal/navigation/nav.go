package navigation

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

// NavNode represents a single item in the navigation tree.
type NavNode struct {
	Title    string
	Slug     string
	FullPath string // URL path: "/docs/guide/getting-started"
	FSPath   string // Absolute filesystem path to .md file (empty for dirs)
	IsDir    bool
	Children []*NavNode
	Depth    int
}

var numericPrefix = regexp.MustCompile(`^\d+[-_]?`)

var frontmatterRe = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?`)

// isDraft returns true if the file starts with "draft: true" in its YAML front matter.
func isDraft(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	m := frontmatterRe.FindSubmatch(data)
	if m == nil {
		return false
	}
	var fm struct {
		Draft bool `yaml:"draft"`
	}
	_ = yaml.Unmarshal(m[1], &fm)
	return fm.Draft
}

// titleFromFrontmatter returns the title from YAML front matter, or empty string.
func titleFromFrontmatter(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	m := frontmatterRe.FindSubmatch(data)
	if m == nil {
		return ""
	}
	var fm struct {
		Title string `yaml:"title"`
	}
	_ = yaml.Unmarshal(m[1], &fm)
	return fm.Title
}

// titleFromSlug converts a filesystem name to a display title.
// "01-getting-started" → "Getting Started"
func titleFromSlug(name string) string {
	// Strip numeric prefix
	name = numericPrefix.ReplaceAllString(name, "")
	// Replace hyphens and underscores with spaces
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	// Title case each word
	words := strings.Fields(name)
	for i, w := range words {
		if len(w) > 0 {
			runes := []rune(w)
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}

// SlugFromName returns the slug portion of a filename (strips numeric prefix, keeps hyphen form).
func SlugFromName(name string) string {
	name = numericPrefix.ReplaceAllString(name, "")
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	decomposed := norm.NFD.String(strings.ToLower(name))
	var b strings.Builder
	prevDash := false
	for _, r := range decomposed {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}

	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "page"
	}
	return slug
}

// BuildTree walks contentDir and builds a NavNode tree.
// basePath is the URL prefix, e.g. "/docs" or "" for root.
// The returned root node represents the content directory itself.
func BuildTree(contentDir, basePath string) (*NavNode, error) {
	siteDir := contentDir

	root := &NavNode{
		Title:    "Home",
		Slug:     "",
		FullPath: basePath + "/",
		FSPath:   "",
		IsDir:    true,
		Depth:    0,
	}

	// dirNodes maps absolute dir path → *NavNode so we can attach children
	dirNodes := map[string]*NavNode{
		siteDir: root,
	}

	err := filepath.WalkDir(siteDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip the root itself
		if path == siteDir {
			return nil
		}

		rel, _ := filepath.Rel(siteDir, path)
		parts := strings.Split(rel, string(os.PathSeparator))
		depth := len(parts)

		name := d.Name()

		// Skip hidden files/dirs and asset dirs (prefix "." or "_")
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		parentPath := filepath.Dir(path)
		parentNode, ok := dirNodes[parentPath]
		if !ok {
			return nil
		}

		if d.IsDir() {
			slug := SlugFromName(name)
			// Build URL path from parts
			urlParts := make([]string, len(parts))
			for i, p := range parts {
				urlParts[i] = SlugFromName(p)
			}
			fullPath := basePath + "/" + strings.Join(urlParts, "/") + "/"

			node := &NavNode{
				Title:    titleFromSlug(name),
				Slug:     slug,
				FullPath: fullPath,
				FSPath:   "",
				IsDir:    true,
				Children: nil,
				Depth:    depth,
			}
			parentNode.Children = append(parentNode.Children, node)
			dirNodes[path] = node
			return nil
		}

		// Only process .md files
		if !strings.HasSuffix(name, ".md") {
			return nil
		}

		baseName := strings.TrimSuffix(name, ".md")
		slug := SlugFromName(baseName)

		// Build URL path
		urlParts := make([]string, len(parts))
		for i, p := range parts {
			if i == len(parts)-1 {
				// Last part is the file
				urlParts[i] = SlugFromName(strings.TrimSuffix(p, ".md"))
			} else {
				urlParts[i] = SlugFromName(p)
			}
		}

		// Skip draft pages
		if isDraft(path) {
			return nil
		}

		var fullPath string
		if baseName == "index" || numericPrefix.ReplaceAllString(baseName, "") == "index" {
			// index.md in a dir → the dir's landing page
			// The parent dir node gets the FSPath set
			parentNode.FSPath = path
			// Override dir title from frontmatter if present
			if t := titleFromFrontmatter(path); t != "" {
				parentNode.Title = t
			}
			return nil
		}

		fullPath = basePath + "/" + strings.Join(urlParts, "/")

		// Use frontmatter title if available, otherwise derive from filename
		title := titleFromFrontmatter(path)
		if title == "" {
			title = titleFromSlug(baseName)
		}

		node := &NavNode{
			Title:    title,
			Slug:     slug,
			FullPath: fullPath,
			FSPath:   path,
			IsDir:    false,
			Depth:    depth,
		}
		parentNode.Children = append(parentNode.Children, node)
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort children: dirs first, then files, each group alphabetically
	sortChildren(root)

	// Remove directories that contain no markdown files (directly or recursively)
	pruneEmpty(root)

	return root, nil
}

// pruneEmpty removes directory nodes that have no markdown content anywhere in their subtree.
func pruneEmpty(node *NavNode) {
	kept := node.Children[:0]
	for _, child := range node.Children {
		if child.IsDir {
			pruneEmpty(child)
			if child.FSPath != "" || len(child.Children) > 0 {
				kept = append(kept, child)
			}
		} else {
			kept = append(kept, child)
		}
	}
	node.Children = kept
}

func sortChildren(node *NavNode) {
	sort.SliceStable(node.Children, func(i, j int) bool {
		a, b := node.Children[i], node.Children[j]
		if a.IsDir != b.IsDir {
			return a.IsDir // dirs first
		}
		return a.Slug < b.Slug
	})
	for _, child := range node.Children {
		sortChildren(child)
	}
}

// FindNode looks up a node by URL path.
func FindNode(root *NavNode, urlPath string) *NavNode {
	// Normalize: strip trailing slash for comparison
	urlPath = strings.TrimRight(urlPath, "/")
	rootPath := strings.TrimRight(root.FullPath, "/")
	if rootPath == urlPath {
		return root
	}
	for _, child := range root.Children {
		if n := FindNode(child, urlPath); n != nil {
			return n
		}
	}
	return nil
}

// FlatPages returns all page nodes (non-dir with FSPath) in sidebar order.
func FlatPages(root *NavNode) []*NavNode {
	var pages []*NavNode
	collectPages(root, &pages)
	return pages
}

func collectPages(node *NavNode, pages *[]*NavNode) {
	if node.FSPath != "" {
		*pages = append(*pages, node)
	}
	for _, child := range node.Children {
		collectPages(child, pages)
	}
}

// PrevNext returns the previous and next page relative to urlPath.
func PrevNext(root *NavNode, urlPath string) (prev, next *NavNode) {
	pages := FlatPages(root)
	urlPath = strings.TrimRight(urlPath, "/")
	for i, p := range pages {
		if strings.TrimRight(p.FullPath, "/") == urlPath {
			if i > 0 {
				prev = pages[i-1]
			}
			if i < len(pages)-1 {
				next = pages[i+1]
			}
			return
		}
	}
	return
}

// Breadcrumbs returns the path from root to the given node.
func Breadcrumbs(root *NavNode, urlPath string) []NavNode {
	var path []NavNode
	if findPath(root, urlPath, &path) {
		return path
	}
	return nil
}

func findPath(node *NavNode, target string, path *[]NavNode) bool {
	norm := strings.TrimRight(node.FullPath, "/")
	t := strings.TrimRight(target, "/")
	*path = append(*path, *node)
	if norm == t {
		return true
	}
	for _, child := range node.Children {
		if findPath(child, target, path) {
			return true
		}
	}
	*path = (*path)[:len(*path)-1]
	return false
}
