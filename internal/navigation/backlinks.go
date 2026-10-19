package navigation

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Backlink represents one incoming reference to a page.
type Backlink struct {
	Title    string // title of the linking page
	FullPath string // URL of the linking page
}

// BacklinkIndex maps page URL → pages that link to it.
type BacklinkIndex map[string][]Backlink

var wikiLinkRe = regexp.MustCompile(`\[\[([^\]|#]+)(?:[|#][^\]]*)?\]\]`)

// BuildBacklinks walks the nav tree and builds an inverted link index.
// basePath is e.g. "/docs".
func BuildBacklinks(root *NavNode, basePath string) BacklinkIndex {
	idx := make(BacklinkIndex)

	// Collect all nodes with filesystem paths for fast lookup by slug path.
	allNodes := make(map[string]*NavNode) // FullPath → node
	files := make(map[string]*NavNode)
	WalkNodes(root, func(n *NavNode) {
		if n.FSPath != "" {
			allNodes[n.FullPath] = n
			files[filepath.Clean(n.FSPath)] = n
		}
	})

	parser := goldmark.New().Parser()
	WalkNodes(root, func(src *NavNode) {
		if src.FSPath == "" {
			return
		}
		data, err := os.ReadFile(src.FSPath)
		if err != nil {
			return
		}
		data = frontmatterRe.ReplaceAll(data, nil)
		var targets []string
		doc := parser.Parse(text.NewReader(data))
		_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			switch node := node.(type) {
			case *ast.Link:
				if target := resolveBacklink(string(node.Destination), src, basePath, allNodes, files); target != "" {
					targets = append(targets, target)
				}
			case *ast.Text:
				if node.Parent().Kind() == ast.KindCodeSpan {
					break
				}
				for _, m := range wikiLinkRe.FindAllStringSubmatch(string(node.Segment.Value(data)), -1) {
					slug := SlugFromName(strings.TrimSpace(m[1]))
					target := ""
					for pageURL := range allNodes {
						if path.Base(pageURL) == slug {
							if target != "" {
								target = ""
								break
							}
							target = pageURL
						}
					}
					if target != "" {
						targets = append(targets, target)
					}
				}
			}
			return ast.WalkContinue, nil
		})
		for _, target := range targets {
			if target == src.FullPath {
				continue // no self-links
			}
			// Deduplicate per source
			already := false
			for _, bl := range idx[target] {
				if bl.FullPath == src.FullPath {
					already = true
					break
				}
			}
			if !already {
				idx[target] = append(idx[target], Backlink{
					Title:    src.Title,
					FullPath: src.FullPath,
				})
			}
		}
	})

	return idx
}

// WalkNodes visits every node in the tree depth-first.
func WalkNodes(node *NavNode, fn func(*NavNode)) {
	if node == nil {
		return
	}
	fn(node)
	for _, ch := range node.Children {
		WalkNodes(ch, fn)
	}
}

// FindNodeByFSPath returns the page backed by fsPath, including directory
// landing pages whose filesystem path belongs to an IsDir node.
func FindNodeByFSPath(root *NavNode, fsPath string) *NavNode {
	cleanPath := filepath.Clean(fsPath)
	var found *NavNode
	WalkNodes(root, func(node *NavNode) {
		if found == nil && node.FSPath != "" && filepath.Clean(node.FSPath) == cleanPath {
			found = node
		}
	})
	return found
}

func resolveBacklink(href string, source *NavNode, basePath string, nodes, files map[string]*NavNode) string {
	u, err := url.Parse(href)
	if err != nil || u.IsAbs() || u.Host != "" || u.Path == "" {
		return ""
	}
	if !strings.HasPrefix(u.Path, "/") {
		if target := files[filepath.Clean(filepath.Join(filepath.Dir(source.FSPath), filepath.FromSlash(u.Path)))]; target != nil {
			return target.FullPath
		}
	}
	resolved := u.Path
	if !strings.HasPrefix(resolved, "/") {
		dir := path.Dir(source.FullPath)
		if source.IsDir {
			dir = source.FullPath
		}
		resolved = path.Join(dir, resolved)
	} else if basePath != "" && resolved != basePath && !strings.HasPrefix(resolved, basePath+"/") {
		resolved = basePath + resolved
	}
	for _, candidate := range []string{resolved, strings.TrimRight(resolved, "/") + "/"} {
		if target := nodes[candidate]; target != nil {
			return target.FullPath
		}
	}
	ext := path.Ext(resolved)
	if !strings.EqualFold(ext, ".md") && !strings.EqualFold(ext, ".markdown") {
		return ""
	}
	resolved = slugNormalizePath(strings.TrimSuffix(resolved, ext))
	for _, candidate := range []string{resolved, resolved + "/", strings.TrimSuffix(resolved, "/index") + "/", strings.TrimSuffix(resolved, "/readme") + "/"} {
		if target := nodes[candidate]; target != nil {
			return target.FullPath
		}
	}
	return ""
}

// slugNormalizePath applies SlugFromName to each non-empty path segment.
func slugNormalizePath(p string) string {
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		if seg != "" {
			segments[i] = SlugFromName(seg)
		}
	}
	return strings.Join(segments, "/")
}
