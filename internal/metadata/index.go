package metadata

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/frontmatter"
	"github.com/mares/dyno/internal/navigation"
)

const QueryPrefix = "meta."

type Filter map[string][]string

type Value struct {
	Value    string
	Count    int
	Selected bool
}

type Facet struct {
	Name   string
	Label  string
	Values []Value
}

type Index struct {
	fields    []config.NamedFrontmatterField
	documents map[string]map[string][]string
}

// Merge creates a library-wide index. Fields with the same name form one
// facet; the first configured label wins and configured option order is merged.
func Merge(indices ...*Index) *Index {
	merged := &Index{documents: make(map[string]map[string][]string)}
	fieldPositions := make(map[string]int)
	for _, idx := range indices {
		if idx == nil {
			continue
		}
		for _, field := range idx.fields {
			if position, exists := fieldPositions[field.Name]; exists {
				current := &merged.fields[position]
				for _, option := range field.Options {
					if !contains(current.Options, option) {
						current.Options = append(current.Options, option)
					}
				}
				continue
			}
			fieldPositions[field.Name] = len(merged.fields)
			merged.fields = append(merged.fields, field)
		}
		for path, document := range idx.documents {
			merged.documents[path] = document
		}
	}
	return merged
}

func Build(root *navigation.NavNode, cfg config.FrontmatterConfig) (*Index, error) {
	idx := &Index{documents: make(map[string]map[string][]string)}
	for _, field := range cfg.OrderedFields() {
		if field.Filterable {
			idx.fields = append(idx.fields, field)
		}
	}
	if len(idx.fields) == 0 {
		return idx, nil
	}

	var buildErr error
	navigation.WalkNodes(root, func(node *navigation.NavNode) {
		if buildErr != nil || node.FSPath == "" {
			return
		}
		source, err := os.ReadFile(node.FSPath)
		if err != nil {
			buildErr = fmt.Errorf("read metadata %s: %w", node.FSPath, err)
			return
		}
		values, err := frontmatter.Read(source)
		if err != nil {
			buildErr = fmt.Errorf("read metadata %s: %w", node.FSPath, err)
			return
		}
		doc := make(map[string][]string)
		for _, field := range idx.fields {
			doc[field.Name] = normalizeValues(values[field.Name])
		}
		idx.documents[node.FullPath] = doc
	})
	return idx, buildErr
}

func (idx *Index) Enabled() bool { return idx != nil && len(idx.fields) > 0 }

func (idx *Index) Parse(values url.Values) Filter {
	filter := make(Filter)
	if idx == nil {
		return filter
	}
	for _, field := range idx.fields {
		seen := make(map[string]bool)
		for _, value := range values[QueryPrefix+field.Name] {
			value = strings.TrimSpace(value)
			if value != "" && !seen[value] {
				filter[field.Name] = append(filter[field.Name], value)
				seen[value] = true
			}
		}
	}
	return filter
}

func (idx *Index) Allowed(filter Filter) map[string]bool {
	if idx == nil || len(filter) == 0 {
		return nil
	}
	allowed := make(map[string]bool)
	for path, document := range idx.documents {
		if matches(document, filter, "") {
			allowed[path] = true
		}
	}
	return allowed
}

func (idx *Index) Count(filter Filter) int {
	if idx == nil {
		return 0
	}
	count := 0
	for _, document := range idx.documents {
		if matches(document, filter, "") {
			count++
		}
	}
	return count
}

func (idx *Index) Facets(filter Filter) []Facet {
	if idx == nil {
		return nil
	}
	facets := make([]Facet, 0, len(idx.fields))
	for _, field := range idx.fields {
		counts := make(map[string]int)
		for _, document := range idx.documents {
			if !matches(document, filter, field.Name) {
				continue
			}
			for _, value := range document[field.Name] {
				counts[value]++
			}
		}
		for _, selected := range filter[field.Name] {
			if _, exists := counts[selected]; !exists {
				counts[selected] = 0
			}
		}
		ordered := orderedValues(field.Options, counts)
		values := make([]Value, 0, len(ordered))
		for _, value := range ordered {
			values = append(values, Value{
				Value: value, Count: counts[value], Selected: contains(filter[field.Name], value),
			})
		}
		label := field.Label
		if label == "" {
			label = field.Name
		}
		facets = append(facets, Facet{Name: field.Name, Label: label, Values: values})
	}
	return facets
}

func Encode(filter Filter) string {
	values := make(url.Values)
	keys := make([]string, 0, len(filter))
	for key := range filter {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range filter[key] {
			values.Add(QueryPrefix+key, value)
		}
	}
	if encoded := values.Encode(); encoded != "" {
		return "?" + encoded
	}
	return ""
}

func matches(document map[string][]string, filter Filter, ignoredField string) bool {
	for field, selected := range filter {
		if field == ignoredField || len(selected) == 0 {
			continue
		}
		matched := false
		for _, wanted := range selected {
			if contains(document[field], wanted) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func normalizeValues(value any) []string {
	var raw []any
	switch typed := value.(type) {
	case nil:
		return nil
	case []any:
		raw = typed
	case []string:
		for _, item := range typed {
			raw = append(raw, item)
		}
	default:
		raw = []any{typed}
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		value := strings.TrimSpace(fmt.Sprint(item))
		if value != "" && !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	return result
}

func orderedValues(configured []string, counts map[string]int) []string {
	seen := make(map[string]bool)
	values := make([]string, 0, len(counts))
	for _, value := range configured {
		if _, exists := counts[value]; exists && !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	rest := make([]string, 0, len(counts)-len(values))
	for value := range counts {
		if !seen[value] {
			rest = append(rest, value)
		}
	}
	sort.Strings(rest)
	return append(values, rest...)
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
