package frontmatter

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var ErrInvalid = errors.New("invalid YAML frontmatter")

type section struct {
	contentStart int
	contentEnd   int
	newline      string
}

type lineReplacement struct {
	start int
	end   int
	lines []string
}

// Read returns all top-level YAML metadata. Documents without frontmatter
// return an empty map.
func Read(source []byte) (map[string]any, error) {
	part, ok := findSection(source)
	if !ok {
		return map[string]any{}, nil
	}
	content := source[part.contentStart:part.contentEnd]
	if len(bytes.TrimSpace(content)) == 0 {
		return map[string]any{}, nil
	}
	var metadata map[string]any
	if err := yaml.Unmarshal(content, &metadata); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	return metadata, nil
}

// Apply updates top-level fields while preserving every untouched YAML block.
// Missing frontmatter is created automatically.
func Apply(source []byte, updates map[string]any) ([]byte, error) {
	if len(updates) == 0 {
		return append([]byte(nil), source...), nil
	}
	part, ok := findSection(source)
	if !ok {
		return addSection(source, updates)
	}

	content := source[part.contentStart:part.contentEnd]
	trimmed := strings.TrimSuffix(string(content), part.newline)
	lines := []string{}
	if trimmed != "" {
		lines = strings.Split(trimmed, part.newline)
	}

	var doc yaml.Node
	if len(bytes.TrimSpace(content)) > 0 {
		if err := yaml.Unmarshal(content, &doc); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	mapping, err := rootMapping(&doc)
	if err != nil {
		return nil, err
	}

	keys := sortedKeys(updates)
	replacements := make([]lineReplacement, 0, len(keys))
	missing := make([]string, 0, len(keys))
	for _, name := range keys {
		keyNode, valueNode := findField(mapping, name)
		if keyNode == nil {
			missing = append(missing, name)
			continue
		}
		if nodeValueEqual(valueNode, updates[name]) {
			continue
		}
		block, err := marshalField(name, updates[name], keyNode, valueNode)
		if err != nil {
			return nil, err
		}
		start := keyNode.Line - 1
		end := nodeEndLine(valueNode) - 1
		if start < 0 || end < start || end >= len(lines) {
			return nil, fmt.Errorf("%w: invalid source positions for %q", ErrInvalid, name)
		}
		replacements = append(replacements, lineReplacement{
			start: start,
			end:   end,
			lines: strings.Split(strings.TrimSuffix(block, "\n"), "\n"),
		})
	}

	sort.Slice(replacements, func(i, j int) bool { return replacements[i].start > replacements[j].start })
	for _, replacement := range replacements {
		updated := make([]string, 0, len(lines)-(replacement.end-replacement.start)+len(replacement.lines))
		updated = append(updated, lines[:replacement.start]...)
		updated = append(updated, replacement.lines...)
		updated = append(updated, lines[replacement.end+1:]...)
		lines = updated
	}
	for _, name := range missing {
		block, err := marshalField(name, updates[name], nil, nil)
		if err != nil {
			return nil, err
		}
		lines = append(lines, strings.Split(strings.TrimSuffix(block, "\n"), "\n")...)
	}

	patched := strings.Join(lines, part.newline)
	if patched != "" {
		patched += part.newline
	}
	out := make([]byte, 0, len(source)+len(patched)-len(content))
	out = append(out, source[:part.contentStart]...)
	out = append(out, patched...)
	out = append(out, source[part.contentEnd:]...)
	return out, nil
}

func findSection(source []byte) (section, bool) {
	firstEnd, firstNewline := nextLine(source, 0)
	if firstEnd < 0 || string(bytes.TrimSuffix(source[:firstEnd], []byte("\r"))) != "---" {
		return section{}, false
	}
	newline := "\n"
	if firstEnd > 0 && source[firstEnd-1] == '\r' {
		newline = "\r\n"
	}
	contentStart := firstEnd + firstNewline
	for start := contentStart; start <= len(source); {
		end, newlineLen := nextLine(source, start)
		if end < 0 {
			break
		}
		line := bytes.TrimSuffix(source[start:end], []byte("\r"))
		if string(line) == "---" {
			return section{contentStart: contentStart, contentEnd: start, newline: newline}, true
		}
		if newlineLen == 0 {
			break
		}
		start = end + newlineLen
	}
	return section{}, false
}

func nextLine(source []byte, start int) (end, newlineLen int) {
	if start > len(source) {
		return -1, 0
	}
	if start == len(source) {
		return start, 0
	}
	idx := bytes.IndexByte(source[start:], '\n')
	if idx < 0 {
		return len(source), 0
	}
	end = start + idx
	return end, 1
}

func addSection(source []byte, updates map[string]any) ([]byte, error) {
	keys := sortedKeys(updates)
	var body strings.Builder
	for _, name := range keys {
		block, err := marshalField(name, updates[name], nil, nil)
		if err != nil {
			return nil, err
		}
		body.WriteString(block)
	}
	return []byte("---\n" + body.String() + "---\n" + string(source)), nil
}

func rootMapping(doc *yaml.Node) (*yaml.Node, error) {
	if len(doc.Content) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: root must be a mapping", ErrInvalid)
	}
	return root, nil
}

func findField(mapping *yaml.Node, name string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			return mapping.Content[i], mapping.Content[i+1]
		}
	}
	return nil, nil
}

func marshalField(name string, value any, oldKey, oldValue *yaml.Node) (string, error) {
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}
	var valueNode yaml.Node
	if err := valueNode.Encode(value); err != nil {
		return "", fmt.Errorf("encode frontmatter field %q: %w", name, err)
	}
	if oldKey != nil {
		key.LineComment = oldKey.LineComment
	}
	if oldValue != nil {
		valueNode.LineComment = oldValue.LineComment
		valueNode.FootComment = oldValue.FootComment
	}
	mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{key, &valueNode}}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
	encoded, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("encode frontmatter field %q: %w", name, err)
	}
	return string(encoded), nil
}

func nodeEndLine(node *yaml.Node) int {
	end := node.Line + strings.Count(node.Value, "\n")
	for _, child := range node.Content {
		if childEnd := nodeEndLine(child); childEnd > end {
			end = childEnd
		}
	}
	return end
}

func nodeValueEqual(node *yaml.Node, value any) bool {
	var current any
	if err := node.Decode(&current); err != nil {
		return false
	}
	currentYAML, currentErr := yaml.Marshal(current)
	valueYAML, valueErr := yaml.Marshal(value)
	return currentErr == nil && valueErr == nil && bytes.Equal(currentYAML, valueYAML)
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
