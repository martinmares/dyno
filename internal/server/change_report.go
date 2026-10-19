package server

import (
	"crypto/sha256"
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strings"
)

type markdownReportBlock struct{ Source, Section, Kind string }
type documentChange struct {
	ID, Kind, Section, BlockType string
	Before, After                template.HTML
}

var reportHeading = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.+?)\s*$`)
var reportClosingHashes = regexp.MustCompile(`[ \t]+#+[ \t]*$`)
var reportList = regexp.MustCompile(`^\s*(?:[-+*]|[0-9]+[.)])\s+`)
var reportReference = regexp.MustCompile(`^ {0,3}\[[^\]]+\]:`)

// Report units are Markdown paragraphs, tables, lists, callouts and fenced
// code. Fences remain whole (including blank lines); headings track context.
func changeReportBlocks(source []byte) []markdownReportBlock {
	body := strings.ReplaceAll(string(stripYAMLFrontmatter(source)), "\r\n", "\n")
	lines := strings.Split(body, "\n")
	var blocks []markdownReportBlock
	var headings [6]string
	for i := 0; i < len(lines); {
		if strings.TrimSpace(lines[i]) == "" {
			i++
			continue
		}
		start := i
		kind := "Text"
		heading := reportHeading.FindStringSubmatch(lines[i])
		setext := i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" && (strings.Trim(strings.TrimSpace(lines[i+1]), "=") == "" || strings.Trim(strings.TrimSpace(lines[i+1]), "-") == "")
		if heading != nil || setext {
			level, title := 1, strings.TrimSpace(lines[i])
			if heading != nil {
				level = len(heading[1])
				title = reportClosingHashes.ReplaceAllString(heading[2], "")
			} else {
				if strings.HasPrefix(strings.TrimSpace(lines[i+1]), "-") {
					level = 2
				}
				i++
			}
			headings[level-1] = title
			for j := level; j < len(headings); j++ {
				headings[j] = ""
			}
			i++
			kind = "Heading"
		} else {
			first := strings.TrimSpace(lines[i])
			grouped := reportList.MatchString(lines[i]) || strings.HasPrefix(first, "??? ") || strings.HasPrefix(first, "!!! ")
			if reportList.MatchString(lines[i]) {
				kind = "List"
			} else if strings.HasPrefix(first, ">") {
				kind = "Quote"
			} else if strings.HasPrefix(first, "??? ") || strings.HasPrefix(first, "!!! ") {
				kind = "Callout"
			}
			var fence byte
			width := 0
			for i < len(lines) {
				trimmed := strings.TrimSpace(lines[i])
				if len(trimmed) >= 3 && (trimmed[0] == '`' || trimmed[0] == '~') {
					count := 0
					for count < len(trimmed) && trimmed[count] == trimmed[0] {
						count++
					}
					if count >= 3 {
						if fence == 0 {
							fence = trimmed[0]
							width = count
							if i == start {
								kind = "Code block"
							}
						} else if trimmed[0] == fence && count >= width && strings.TrimSpace(trimmed[count:]) == "" {
							fence = 0
						}
					}
				}
				if i > start && fence == 0 && reportHeading.MatchString(lines[i]) {
					break
				}
				if fence == 0 && trimmed == "" {
					next := i + 1
					for next < len(lines) && strings.TrimSpace(lines[next]) == "" {
						next++
					}
					if !grouped || next == len(lines) || (!reportList.MatchString(lines[next]) && !strings.HasPrefix(lines[next], " ") && !strings.HasPrefix(lines[next], "\t")) {
						break
					}
				}
				i++
			}
			if i == start {
				i++
			}
		}
		text := strings.TrimRight(strings.Join(lines[start:i], "\n"), "\n")
		if kind == "Text" && strings.Contains(text, "|") && strings.Contains(text, "---") {
			kind = "Table"
		}
		var context []string
		for _, heading := range headings {
			if heading != "" {
				context = append(context, heading)
			}
		}
		if len(context) > 1 && headings[0] != "" {
			context = context[1:]
		}
		section := strings.Join(context, " / ")
		if section == "" {
			section = "Introduction"
		}
		blocks = append(blocks, markdownReportBlock{Source: text, Section: section, Kind: kind})
	}
	return blocks
}

func (s *Server) buildChangeReport(left, right []byte, sourcePath string) ([]documentChange, int, int, int, error) {
	before, after := changeReportBlocks(left), changeReportBlocks(right)
	hashes := func(blocks []markdownReportBlock) string {
		var lines []string
		for _, block := range blocks {
			lines = append(lines, fmt.Sprintf("%x", sha256.Sum256([]byte(block.Source))))
		}
		return strings.Join(lines, "\n")
	}
	rows, _, _, _ := buildSideBySideDiff(hashes(before), hashes(after))
	references := func(blocks []markdownReportBlock) string {
		var definitions []string
		for _, block := range blocks {
			if reportReference.MatchString(block.Source) {
				definitions = append(definitions, block.Source)
			}
		}
		return strings.Join(definitions, "\n\n")
	}
	leftRefs, rightRefs := references(before), references(after)
	render := func(block markdownReportBlock, refs string) (template.HTML, error) {
		result, err := s.renderer.Render([]byte(block.Source + "\n\n" + refs + "\n"))
		if err != nil {
			return "", err
		}
		content := s.rewriteMDLinks(result.HTML, sourcePath)
		content = rewriteAbsoluteLinks(content, s.basePath)
		if strings.TrimSpace(content) == "" {
			content = "<pre>" + html.EscapeString(block.Source) + "</pre>"
		}
		return template.HTML(content), nil
	}
	var changes []documentChange
	added, removed, changed := 0, 0, 0
	appendChange := func(old, new *markdownReportBlock) error {
		change := documentChange{ID: fmt.Sprintf("change-%d", len(changes)+1)}
		if old != nil {
			change.Section = old.Section
			change.BlockType = old.Kind
			content, err := render(*old, leftRefs)
			if err != nil {
				return err
			}
			change.Before = content
		}
		if new != nil {
			change.Section = new.Section
			change.BlockType = new.Kind
			content, err := render(*new, rightRefs)
			if err != nil {
				return err
			}
			change.After = content
		}
		if old == nil {
			change.Kind = "Added"
			added++
		} else if new == nil {
			change.Kind = "Removed"
			removed++
		} else {
			change.Kind = "Changed"
			changed++
		}
		changes = append(changes, change)
		return nil
	}
	for _, row := range rows {
		if !row.Changed {
			continue
		}
		var old, new *markdownReportBlock
		if row.LeftLine > 0 && row.LeftLine <= len(before) {
			old = &before[row.LeftLine-1]
		}
		if row.RightLine > 0 && row.RightLine <= len(after) {
			new = &after[row.RightLine-1]
		}
		if old != nil && new != nil && old.Kind != new.Kind {
			if err := appendChange(old, nil); err != nil {
				return nil, 0, 0, 0, err
			}
			old = nil
		}
		if old != nil || new != nil {
			if err := appendChange(old, new); err != nil {
				return nil, 0, 0, 0, err
			}
		}
	}
	return changes, added, removed, changed, nil
}
