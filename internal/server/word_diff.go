package server

import (
	"bytes"
	"html"
	"html/template"
	"regexp"
	"strings"

	htmlnode "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type textRange struct{ start, end int }

var diffWords = regexp.MustCompile(`[\p{L}\p{M}\p{N}_]+|\s+|[^\p{L}\p{M}\p{N}_\s]`)

// Use byte ranges into the original strings so highlighting never changes text.
// Bound the LCS matrix; very large blocks retain coarse highlighting instead.
func changedWordRanges(left, right string) ([]textRange, []textRange) {
	a, b := diffWords.FindAllStringIndex(left, -1), diffWords.FindAllStringIndex(right, -1)
	key := func(text string, token []int) string {
		value := text[token[0]:token[1]]
		if strings.TrimSpace(value) == "" {
			return " "
		}
		return value
	}
	// Exclude common context before allocating the matrix, especially long lists.
	for len(a) > 0 && len(b) > 0 && key(left, a[0]) == key(right, b[0]) {
		a, b = a[1:], b[1:]
	}
	for len(a) > 0 && len(b) > 0 && key(left, a[len(a)-1]) == key(right, b[len(b)-1]) {
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	if len(a)*len(b) > 1_000_000 {
		return []textRange{{a[0][0], a[len(a)-1][1]}}, []textRange{{b[0][0], b[len(b)-1][1]}}
	}
	width := len(b) + 1
	dp := make([]uint32, (len(a)+1)*width)
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if key(left, a[i]) == key(right, b[j]) {
				dp[i*width+j] = dp[(i+1)*width+j+1] + 1
			} else {
				dp[i*width+j] = max(dp[(i+1)*width+j], dp[i*width+j+1])
			}
		}
	}
	var removed, added []textRange
	appendRange := func(ranges *[]textRange, token []int) {
		if n := len(*ranges); n > 0 && (*ranges)[n-1].end == token[0] {
			(*ranges)[n-1].end = token[1]
		} else {
			*ranges = append(*ranges, textRange{token[0], token[1]})
		}
	}
	for i, j := 0, 0; i < len(a) || j < len(b); {
		if i < len(a) && j < len(b) && key(left, a[i]) == key(right, b[j]) {
			i++
			j++
		} else if j == len(b) || (i < len(a) && dp[(i+1)*width+j] >= dp[i*width+j+1]) {
			appendRange(&removed, a[i])
			i++
		} else {
			appendRange(&added, b[j])
			j++
		}
	}
	return removed, added
}

func highlightedSource(text string, ranges []textRange, kind string) template.HTML {
	var out strings.Builder
	position := 0
	for _, r := range ranges {
		out.WriteString(html.EscapeString(text[position:r.start]))
		out.WriteString(`<span class="dyno-diff-word dyno-diff-word-` + kind + `">`)
		out.WriteString(html.EscapeString(text[r.start:r.end]))
		out.WriteString(`</span>`)
		position = r.end
	}
	out.WriteString(html.EscapeString(text[position:]))
	return template.HTML(out.String())
}

func highlightSourceRows(rows []gitCompareRow) {
	for i := range rows {
		row := &rows[i]
		var removed, added []textRange
		switch row.Kind {
		case "change":
			removed, added = changedWordRanges(row.LeftText, row.RightText)
		case "delete":
			removed = []textRange{{0, len(row.LeftText)}}
		case "add":
			added = []textRange{{0, len(row.RightText)}}
		}
		row.LeftHTML = highlightedSource(row.LeftText, removed, "removed")
		row.RightHTML = highlightedSource(row.RightText, added, "added")
	}
}

type reportTextNode struct {
	node   *htmlnode.Node
	offset int
}

func reportFragment(content template.HTML) (*htmlnode.Node, string, []reportTextNode, error) {
	root := &htmlnode.Node{Type: htmlnode.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := htmlnode.ParseFragment(strings.NewReader(string(content)), root)
	if err != nil {
		return nil, "", nil, err
	}
	for _, node := range nodes {
		root.AppendChild(node)
	}
	var text strings.Builder
	var texts []reportTextNode
	var walk func(*htmlnode.Node)
	walk = func(node *htmlnode.Node) {
		if node.Type == htmlnode.ElementNode {
			switch node.Data {
			case "script", "style", "svg", "math":
				return
			}
		}
		if node.Type == htmlnode.TextNode {
			texts = append(texts, reportTextNode{node, text.Len()})
			text.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return root, text.String(), texts, nil
}

func markReportText(root *htmlnode.Node, nodes []reportTextNode, ranges []textRange, kind string) (template.HTML, error) {
	for _, entry := range nodes {
		node := entry.node
		position := 0
		for _, r := range ranges {
			start, end := max(0, r.start-entry.offset), min(len(node.Data), r.end-entry.offset)
			if start >= end {
				continue
			}
			if start > position {
				node.Parent.InsertBefore(&htmlnode.Node{Type: htmlnode.TextNode, Data: node.Data[position:start]}, node)
			}
			mark := &htmlnode.Node{Type: htmlnode.ElementNode, Data: "span", DataAtom: atom.Span, Attr: []htmlnode.Attribute{{Key: "class", Val: "dyno-diff-word dyno-diff-word-" + kind}}}
			mark.AppendChild(&htmlnode.Node{Type: htmlnode.TextNode, Data: node.Data[start:end]})
			node.Parent.InsertBefore(mark, node)
			position = end
		}
		if position > 0 {
			if position < len(node.Data) {
				node.Parent.InsertBefore(&htmlnode.Node{Type: htmlnode.TextNode, Data: node.Data[position:]}, node)
			}
			// Preserve server highlighting: the client must not replace diff marks in code.
			for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
				if ancestor.Data == "code" && ancestor.Parent != nil && ancestor.Parent.Data == "pre" {
					found := false
					for i := range ancestor.Attr {
						if ancestor.Attr[i].Key == "class" {
							if !strings.Contains(ancestor.Attr[i].Val, "dyno-code-highlight") {
								ancestor.Attr[i].Val += " dyno-code-highlight"
							}
							found = true
							break
						}
					}
					if !found {
						ancestor.Attr = append(ancestor.Attr, htmlnode.Attribute{Key: "class", Val: "dyno-code-highlight"})
					}
					break
				}
			}
			node.Parent.RemoveChild(node)
		}
	}
	var out bytes.Buffer
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if err := htmlnode.Render(&out, child); err != nil {
			return "", err
		}
	}
	return template.HTML(out.String()), nil
}

func highlightReportPair(before, after template.HTML) (template.HTML, template.HTML, error) {
	left, leftText, leftNodes, err := reportFragment(before)
	if err != nil {
		return "", "", err
	}
	right, rightText, rightNodes, err := reportFragment(after)
	if err != nil {
		return "", "", err
	}
	removed, added := changedWordRanges(leftText, rightText)
	old, err := markReportText(left, leftNodes, removed, "removed")
	if err != nil {
		return "", "", err
	}
	new, err := markReportText(right, rightNodes, added, "added")
	return old, new, err
}
