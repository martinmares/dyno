// Package browse describes an explicit selection of local Markdown sources.
package browse

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mares/dyno/internal/navigation"
)

const BasePath = "/browse"

type source struct {
	path, dir, id, label string
	file                 bool
	root                 *os.Root
}

type Catalog struct {
	sources []*source
	filter  navigation.Filter
}

// New resolves explicitly selected symlinks once. Descendant symlinks are never
// discovered; opened roots confine subsequent file access even during renames.
func New(paths, excludes []string) (*Catalog, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	c := &Catalog{filter: navigation.Filter{Exclude: excludes}}
	for _, pattern := range excludes {
		if _, err := path.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("invalid --exclude %q: %w", pattern, err)
		}
	}
	for _, input := range paths {
		abs, err := filepath.Abs(input)
		if err != nil {
			return nil, err
		}
		abs, err = filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("browse %q: %w", input, err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() && (!info.Mode().IsRegular() || !IsMarkdown(abs)) {
			return nil, fmt.Errorf("browse %q: expected a directory or Markdown file", input)
		}
		s := &source{path: abs, dir: abs, file: !info.IsDir()}
		if s.file {
			s.dir = filepath.Dir(abs)
		}
		sum := sha256.Sum256([]byte(abs))
		s.id = fmt.Sprintf("%x", sum[:8])
		duplicate := false
		for _, previous := range c.sources {
			if previous.path == s.path || (!previous.file && c.covers(previous.path, s.path)) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		if !s.file {
			kept := c.sources[:0]
			for _, previous := range c.sources {
				if !c.covers(s.path, previous.path) {
					kept = append(kept, previous)
				}
			}
			c.sources = kept
		}
		c.sources = append(c.sources, s)
	}
	for _, s := range c.sources {
		root, err := os.OpenRoot(s.dir)
		if err != nil {
			c.Close()
			return nil, err
		}
		s.root = root
		s.label = filepath.Base(s.path)
		for depth := 1; depth < len(strings.Split(filepath.ToSlash(s.path), "/")); depth++ {
			unique := true
			for _, other := range c.sources {
				if other != s && suffix(other.path, depth) == suffix(s.path, depth) {
					unique = false
					break
				}
			}
			if unique {
				s.label = suffix(s.path, depth)
				break
			}
		}
	}
	return c, nil
}

func suffix(name string, depth int) string {
	parts := strings.Split(filepath.ToSlash(name), "/")
	if depth > len(parts) {
		depth = len(parts)
	}
	return strings.Join(parts[len(parts)-depth:], "/")
}

func (c *Catalog) Close() {
	for _, s := range c.sources {
		if s.root != nil {
			_ = s.root.Close()
		}
	}
}

func IsMarkdown(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".md" || ext == ".markdown"
}

func within(root, name string) bool {
	rel, err := filepath.Rel(root, name)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (c *Catalog) covers(root, name string) bool {
	if !within(root, name) {
		return false
	}
	rel, _ := filepath.Rel(root, name)
	return rel == "." || c.allowed(rel)
}

var ignored = map[string]bool{"node_modules": true, "vendor": true, "target": true, "dist": true, "build": true, "venv": true, "__pycache__": true}

func (c *Catalog) allowed(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ".") || ignored[part] || !c.filter.Allows(strings.Join(parts[:i+1], "/")) {
			return false
		}
	}
	return true
}

func safeFile(s *source, rel string) (os.FileInfo, error) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i := range parts {
		info, err := s.root.Lstat(filepath.FromSlash(strings.Join(parts[:i+1], "/")))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, os.ErrPermission
		}
		if i == len(parts)-1 {
			if !info.Mode().IsRegular() {
				return nil, os.ErrPermission
			}
			return info, nil
		}
	}
	return nil, os.ErrPermission
}

func (c *Catalog) document(name string) (*source, string, error) {
	name = filepath.Clean(name)
	if !IsMarkdown(name) {
		return nil, "", os.ErrPermission
	}
	for _, s := range c.sources {
		if (s.file && name != s.path) || (!s.file && !within(s.dir, name)) {
			continue
		}
		rel, _ := filepath.Rel(s.dir, name)
		if !s.file && !c.allowed(rel) {
			continue
		}
		if _, err := safeFile(s, rel); err != nil {
			return nil, "", err
		}
		return s, rel, nil
	}
	return nil, "", os.ErrPermission
}

func (c *Catalog) ReadFile(name string) ([]byte, error) {
	s, rel, err := c.document(name)
	if err != nil {
		return nil, err
	}
	return s.root.ReadFile(rel)
}

func (c *Catalog) Stat(name string) (os.FileInfo, error) {
	s, rel, err := c.document(name)
	if err != nil {
		return nil, err
	}
	return s.root.Stat(rel)
}

func (c *Catalog) WriteFile(name string, data []byte, mode os.FileMode) error {
	s, rel, err := c.document(name)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(rel), ".dyno-edit-"+rand.Text())
	f, err := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer s.root.Remove(tmp)
	if err = f.Chmod(mode.Perm()); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return s.root.Rename(tmp, rel)
}

// Build preserves filesystem names and keeps README/index as ordinary files.
// The signature includes attachments, so their changes also refresh readers.
func (c *Catalog) Build() (*navigation.NavNode, string, error) {
	root := &navigation.NavNode{Title: "Files", FullPath: BasePath + "/", IsDir: true, Navigable: true}
	hash := sha256.New()
	for _, s := range c.sources {
		mount := &navigation.NavNode{Title: s.label, SourceName: filepath.Base(s.path), FullPath: BasePath + "/" + s.id + "/", IsDir: !s.file, Navigable: true, Depth: 1}
		if s.file {
			if info, err := safeFile(s, filepath.Base(s.path)); err == nil {
				mount.FSPath = s.path
				mount.FullPath += url.PathEscape(filepath.Base(s.path))
				root.Children = append(root.Children, mount)
				fmt.Fprintf(hash, "%s:%d:%d;", s.path, info.ModTime().UnixNano(), info.Size())
			}
			continue
		}
		dirs := map[string]*navigation.NavNode{".": mount}
		err := fs.WalkDir(s.root.FS(), ".", func(rel string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			if !c.allowed(rel) {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			parent := dirs[filepath.ToSlash(filepath.Dir(rel))]
			if parent == nil {
				return nil
			}
			node := &navigation.NavNode{Title: entry.Name(), SourceName: entry.Name(), FullPath: mount.FullPath + escapePath(rel), Depth: parent.Depth + 1, IsDir: entry.IsDir(), Navigable: true}
			if entry.IsDir() {
				node.FullPath += "/"
				dirs[rel] = node
			} else {
				info, err := entry.Info()
				if err != nil {
					return err
				}
				if !info.Mode().IsRegular() {
					return nil
				}
				fmt.Fprintf(hash, "%s:%s:%d:%d;", s.id, rel, info.ModTime().UnixNano(), info.Size())
				if !IsMarkdown(rel) {
					return nil
				}
				node.FSPath = filepath.Join(s.dir, filepath.FromSlash(rel))
			}
			parent.Children = append(parent.Children, node)
			return nil
		})
		if err != nil {
			return nil, "", err
		}
		pruneAndSort(mount)
		root.Children = append(root.Children, mount)
	}
	return root, fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func pruneAndSort(node *navigation.NavNode) {
	kept := node.Children[:0]
	for _, child := range node.Children {
		if child.IsDir {
			pruneAndSort(child)
			if len(child.Children) == 0 {
				continue
			}
		}
		kept = append(kept, child)
	}
	node.Children = kept
	sort.SliceStable(node.Children, func(i, j int) bool {
		a, b := node.Children[i], node.Children[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return naturalLess(a.Title, b.Title)
	})
}

func naturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for len(a) > 0 && len(b) > 0 {
		if a[0] >= '0' && a[0] <= '9' && b[0] >= '0' && b[0] <= '9' {
			i, j := 0, 0
			for i < len(a) && a[i] >= '0' && a[i] <= '9' {
				i++
			}
			for j < len(b) && b[j] >= '0' && b[j] <= '9' {
				j++
			}
			x, y := strings.TrimLeft(a[:i], "0"), strings.TrimLeft(b[:j], "0")
			if len(x) != len(y) {
				return len(x) < len(y)
			}
			if x != y {
				return x < y
			}
			a, b = a[i:], b[j:]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

// Resolve resolves a local URL path relative to its document. Absolute URL
// paths are relative to that source's selected root, never the entire disk.
func (c *Catalog) Resolve(document, reference string) (string, error) {
	s, _, err := c.document(document)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(reference, "\\\x00") {
		return "", os.ErrPermission
	}
	if strings.HasPrefix(reference, "/") {
		return filepath.Join(s.dir, filepath.FromSlash(strings.TrimLeft(reference, "/"))), nil
	}
	return filepath.Clean(filepath.Join(filepath.Dir(document), filepath.FromSlash(reference))), nil
}

func (c *Catalog) NavigationPath(name string) string {
	for _, s := range c.sources {
		if s.file || !within(s.dir, name) {
			continue
		}
		rel, _ := filepath.Rel(s.dir, name)
		if rel == "." {
			return BasePath + "/" + s.id + "/"
		}
		if c.allowed(rel) {
			return BasePath + "/" + s.id + "/" + escapePath(filepath.ToSlash(rel))
		}
	}
	return ""
}

func escapePath(rel string) string {
	parts := strings.Split(rel, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

// OpenAsset permits attachments within selected directories. File-only sources
// permit referenced images under the document's parent, but no other files.
func (c *Catalog) OpenAsset(document, name string) (*os.File, error) {
	s, _, err := c.document(document)
	if err != nil {
		return nil, err
	}
	for _, candidate := range c.sources {
		if !candidate.file && c.covers(candidate.dir, name) {
			s = candidate
			break
		}
	}
	if s.file {
		switch strings.ToLower(filepath.Ext(name)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".avif", ".bmp", ".ico":
		default:
			return nil, os.ErrPermission
		}
	}
	if !within(s.dir, name) || IsMarkdown(name) {
		return nil, os.ErrPermission
	}
	rel, _ := filepath.Rel(s.dir, name)
	if !c.allowed(rel) {
		return nil, os.ErrPermission
	}
	if _, err := safeFile(s, rel); err != nil {
		return nil, err
	}
	return s.root.Open(rel)
}
