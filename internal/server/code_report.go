package server

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type reportCodeDiff struct{ Hunks []reportCodeHunk }
type reportCodeHunk struct {
	Location      string
	Before, After template.HTML
	MarkupOnly    bool
}

func reportCodeNode(content template.HTML) (*html.Node, string, error) {
	root, _, _, err := reportFragment(content)
	if err != nil {
		return nil, "", err
	}
	var code *html.Node
	count := 0
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "code" && node.Parent != nil && node.Parent.Data == "pre" {
			code = node
			count++
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if count != 1 {
		return nil, "", nil
	}
	return code, tableNodeText(code), nil
}

// Clone the selected code text range without dropping syntax or word-diff spans.
func codeTextSlice(node *html.Node, start, end int, offset *int) *html.Node {
	copy := &html.Node{Type: node.Type, Data: node.Data, DataAtom: node.DataAtom, Namespace: node.Namespace, Attr: append([]html.Attribute(nil), node.Attr...)}
	if node.Type == html.TextNode {
		from, to := max(0, start-*offset), min(len(node.Data), end-*offset)
		*offset += len(node.Data)
		if from >= to {
			return nil
		}
		copy.Data = node.Data[from:to]
		return copy
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if clipped := codeTextSlice(child, start, end, offset); clipped != nil {
			copy.AppendChild(clipped)
		}
	}
	if copy.FirstChild == nil {
		return nil
	}
	return copy
}

func renderCodeExcerpt(code *html.Node, text string, first, last int) (template.HTML, error) {
	if first == 0 {
		return "", nil
	}
	lines := splitDiffLines(text)
	start := 0
	for _, line := range lines[:first-1] {
		start += len(line) + 1
	}
	end := start
	for i := first - 1; i < last; i++ {
		end += len(lines[i])
		if i < last-1 {
			end++
		}
	}
	offset := 0
	clipped := codeTextSlice(code, start, end, &offset)
	if clipped == nil {
		clipped = &html.Node{Type: html.ElementNode, Data: "code", DataAtom: atom.Code}
	}
	class := "dyno-report-code-excerpt"
	for _, attr := range code.Parent.Attr {
		if attr.Key == "class" {
			class += " " + attr.Val
		}
	}
	pre := &html.Node{Type: html.ElementNode, Data: "pre", DataAtom: atom.Pre, Attr: []html.Attribute{{Key: "class", Val: class}}}
	pre.AppendChild(clipped)
	var out bytes.Buffer
	if err := html.Render(&out, pre); err != nil {
		return "", err
	}
	return template.HTML(out.String()), nil
}

func buildCodeReport(before, after template.HTML) (*reportCodeDiff, error) {
	left, leftText, err := reportCodeNode(before)
	if err != nil {
		return nil, err
	}
	right, rightText, err := reportCodeNode(after)
	if err != nil {
		return nil, err
	}
	if left == nil || right == nil {
		return nil, nil
	}
	rows, _, _, _ := buildSideBySideDiff(leftText, rightText)
	keep := make([]bool, len(rows))
	changed := false
	for i, row := range rows {
		if row.Changed {
			changed = true
			for j := max(0, i-2); j < min(len(rows), i+3); j++ {
				keep[j] = true
			}
		}
	}
	omitted := false
	for _, selected := range keep {
		if !selected {
			omitted = true
			break
		}
	}
	if !changed || !omitted {
		return nil, nil
	}
	result := &reportCodeDiff{}
	for i := 0; i < len(rows); {
		if !keep[i] {
			i++
			continue
		}
		var firstLeft, lastLeft, firstRight, lastRight int
		for i < len(rows) && keep[i] {
			if line := rows[i].LeftLine; line > 0 {
				if firstLeft == 0 {
					firstLeft = line
				}
				lastLeft = line
			}
			if line := rows[i].RightLine; line > 0 {
				if firstRight == 0 {
					firstRight = line
				}
				lastRight = line
			}
			i++
		}
		hunk := reportCodeHunk{}
		var locations []string
		if firstLeft > 0 {
			locations = append(locations, fmt.Sprintf("Before lines %d-%d", firstLeft, lastLeft))
		}
		if firstRight > 0 {
			locations = append(locations, fmt.Sprintf("After lines %d-%d", firstRight, lastRight))
		}
		hunk.Location = strings.Join(locations, "; ")
		hunk.Before, err = renderCodeExcerpt(left, leftText, firstLeft, lastLeft)
		if err != nil {
			return nil, err
		}
		hunk.After, err = renderCodeExcerpt(right, rightText, firstRight, lastRight)
		if err != nil {
			return nil, err
		}
		result.Hunks = append(result.Hunks, hunk)
	}
	return result, nil
}
