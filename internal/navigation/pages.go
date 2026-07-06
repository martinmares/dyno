package navigation

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type pagesNavItem struct {
	Target   string
	Title    string
	Wildcard bool
}

func applyPagesFiles(node *NavNode, dir string) {
	filename := filepath.Join(dir, ".pages")
	items, configured, err := loadPagesNav(filename)
	if err != nil {
		slog.Warn("ignoring invalid .pages file", "file", filename, "err", err)
	} else if configured {
		applyPagesNav(node, dir, items)
	}

	for _, child := range node.Children {
		if !child.IsDir {
			continue
		}
		applyPagesFiles(child, filepath.Join(dir, child.SourceName))
	}
}

func loadPagesNav(filename string) ([]pagesNavItem, bool, error) {
	data, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", filename, err)
	}

	var document struct {
		Nav yaml.Node `yaml:"nav"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", filename, err)
	}
	if document.Nav.Kind == 0 {
		return nil, false, nil
	}
	if document.Nav.Kind != yaml.SequenceNode {
		return nil, false, fmt.Errorf("parse %s: nav must be a list", filename)
	}

	items := make([]pagesNavItem, 0, len(document.Nav.Content))
	for i, entry := range document.Nav.Content {
		switch entry.Kind {
		case yaml.ScalarNode:
			target := strings.TrimSpace(entry.Value)
			if target == "" {
				return nil, false, fmt.Errorf("parse %s: nav[%d] is empty", filename, i)
			}
			items = append(items, pagesNavItem{Target: target, Wildcard: target == "..."})
		case yaml.MappingNode:
			if len(entry.Content) != 2 || entry.Content[0].Kind != yaml.ScalarNode || entry.Content[1].Kind != yaml.ScalarNode {
				return nil, false, fmt.Errorf("parse %s: nav[%d] must contain one title-to-path mapping", filename, i)
			}
			title := strings.TrimSpace(entry.Content[0].Value)
			target := strings.TrimSpace(entry.Content[1].Value)
			if title == "" || target == "" {
				return nil, false, fmt.Errorf("parse %s: nav[%d] title and path are required", filename, i)
			}
			items = append(items, pagesNavItem{Target: target, Title: title})
		default:
			return nil, false, fmt.Errorf("parse %s: nav[%d] must be a path or title-to-path mapping", filename, i)
		}
	}
	return items, true, nil
}

func applyPagesNav(node *NavNode, dir string, items []pagesNavItem) {
	filename := filepath.Join(dir, ".pages")
	children := make(map[string]*NavNode, len(node.Children))
	for _, child := range node.Children {
		children[filepath.ToSlash(child.SourceName)] = child
	}

	// Explicit entries are removed from the automatic group before assembling
	// the result. This lets entries after "..." remain after that placeholder.
	explicit := make(map[string]bool, len(items))
	for _, item := range items {
		if item.Wildcard {
			continue
		}
		target, ok := validPagesTarget(item.Target)
		if !ok {
			continue
		}
		if _, ok := children[target]; ok {
			explicit[target] = true
		}
	}

	ordered := make([]*NavNode, 0, len(node.Children))
	appended := make(map[string]bool, len(node.Children))
	appendAutomatic := func() {
		for _, child := range node.Children {
			target := filepath.ToSlash(child.SourceName)
			if explicit[target] || appended[target] {
				continue
			}
			ordered = append(ordered, child)
			appended[target] = true
		}
	}

	wildcardSeen := false
	for _, item := range items {
		if item.Wildcard {
			if !wildcardSeen {
				appendAutomatic()
				wildcardSeen = true
			}
			continue
		}
		target, ok := validPagesTarget(item.Target)
		if !ok {
			slog.Warn("ignoring invalid .pages navigation target", "file", filename, "target", item.Target)
			continue
		}

		// Landing pages belong to the current directory node, not its children.
		if node.FSPath != "" && strings.EqualFold(filepath.Base(node.FSPath), target) {
			if item.Title != "" {
				node.Title = item.Title
			}
			continue
		}

		child, ok := children[target]
		if !ok {
			// A Dyno content filter or draft frontmatter may intentionally omit a
			// real file. Only warn for targets that do not exist on disk.
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(target))); os.IsNotExist(err) {
				slog.Warn("ignoring missing .pages navigation target", "file", filename, "target", item.Target)
			}
			continue
		}
		if appended[target] {
			slog.Warn("ignoring duplicate .pages navigation target", "file", filename, "target", item.Target)
			continue
		}
		if item.Title != "" {
			child.Title = item.Title
		}
		ordered = append(ordered, child)
		appended[target] = true
	}

	// Dyno deliberately differs from awesome-pages here: .pages is navigation
	// metadata, never a publication filter. Unlisted children always remain.
	appendAutomatic()
	node.Children = ordered
}

func validPagesTarget(value string) (string, bool) {
	target := filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
	return target, target != "." && !strings.HasPrefix(target, "../") && !strings.Contains(target, "/")
}
