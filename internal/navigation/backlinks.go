package navigation

import (
	"net/url"
	"os"
	"path"
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

var markdownLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(<?(\.?\.?/[^)>]*\.md)[>]?>\)`)
var wikiLinkRe = regexp.MustCompile(`\[\[([^\]|#]+)(?:[|#][^\]]*)?\]\]`)

// BuildBacklinks walks the nav tree and builds an inverted link index.
// basePath is e.g. "/docs".
func BuildBacklinks(root *NavNode, basePath string) BacklinkIndex {
	idx := make(BacklinkIndex)

	// Collect all nodes with filesystem paths for fast lookup by slug path.
	allNodes := make(map[string]*NavNode) // FullPath → node
	WalkNodes(root, func(n *NavNode) {
		if n.FSPath != "" {
			allNodes[n.FullPath] = n
		}
	})

	WalkNodes(root, func(src *NavNode) {
		if src.FSPath == "" {
			return
		}
		data, err := os.ReadFile(src.FSPath)
		if err != nil {
			return
		}
		srcDir := path.Dir(src.FullPath)

		targets := extractLinkTargets(string(data), srcDir, allNodes)
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

// extractLinkTargets returns resolved page URLs linked from src markdown content.
func extractLinkTargets(src, srcDir string, allNodes map[string]*NavNode) []string {
	var targets []string

	// Standard markdown links: [text](<./foo.md>) or [text](./foo.md)
	for _, m := range markdownLinkRe.FindAllStringSubmatch(src, -1) {
		href := m[2]
		decoded, err := url.PathUnescape(href)
		if err != nil {
			decoded = href
		}
		decoded = strings.TrimSuffix(decoded, ".md")
		var resolved string
		if strings.HasPrefix(decoded, "/") {
			resolved = decoded
		} else {
			resolved = path.Join(srcDir, decoded)
		}
		resolved = slugNormalizePath(resolved)
		if _, ok := allNodes[resolved]; ok {
			targets = append(targets, resolved)
		}
	}

	// Obsidian wiki-links: [[Page Name]] or [[Page Name|alias]]
	for _, m := range wikiLinkRe.FindAllStringSubmatch(src, -1) {
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		// Try to find the page by slug anywhere in the tree
		slug := SlugFromName(name)
		for pageURL := range allNodes {
			if path.Base(pageURL) == slug {
				targets = append(targets, pageURL)
				break
			}
		}
	}

	return targets
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
