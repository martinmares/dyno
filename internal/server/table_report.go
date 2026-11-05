package server

import (
	"bytes"
	"fmt"
	"html/template"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

type reportTable struct {
	Headers []string
	Rows    [][]template.HTML
}

type reportTableDiff struct {
	Note    string
	Rows    []reportTableRow
	Context []reportTableContext
}

type reportTableContext struct {
	Side, Column, Headers string
	Rows                  []reportTableContextRow
}

type reportTableContextRow struct {
	Number                          int
	Identity, Kind, Columns, Anchor string
	Gap                             bool
}

type reportTableRow struct {
	Identity, Kind     string
	Cells              []reportTableCell
	Anchor             string
	OldIndex, NewIndex int
}

type reportTableCell struct {
	Column        string
	Before, After template.HTML
	MarkupOnly    bool
}

func tableNodeText(node *html.Node) string {
	if node.Type == html.ElementNode && node.Data == "br" {
		return "\n"
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var out strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		out.WriteString(tableNodeText(child))
	}
	return out.String()
}

func reportNodeHasClass(node *html.Node, class string) bool {
	for _, attr := range node.Attr {
		if attr.Key == "class" && slices.Contains(strings.Fields(attr.Val), class) {
			return true
		}
	}
	return false
}

func singleReportElement(root *html.Node) *html.Node {
	var element *html.Node
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode && strings.TrimSpace(child.Data) == "" {
			continue
		}
		if child.Type != html.ElementNode || element != nil {
			return nil
		}
		element = child
	}
	return element
}

// Only convert standalone, rectangular tables. Preserve the ordinary report
// rendering for complex hand-written HTML (nested tables, row/column spans).
func parseReportTable(content template.HTML) (*reportTable, error) {
	if content == "" {
		return nil, nil
	}
	root, _, _, err := reportFragment(content)
	if err != nil {
		return nil, err
	}
	table := singleReportElement(root)
	if table != nil && table.Data == "div" && reportNodeHasClass(table, "dyno-table-widget") {
		// Interactive rendering adds controls outside the actual table. They are
		// not document content and must not prevent cell-level report generation.
		var responsive *html.Node
		for child := table.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && reportNodeHasClass(child, "dyno-table-responsive") {
				if responsive != nil {
					return nil, nil
				}
				responsive = child
			}
		}
		if responsive == nil {
			return nil, nil
		}
		table = singleReportElement(responsive)
	}
	if table == nil || table.Data != "table" {
		return nil, nil
	}
	result := &reportTable{}
	valid := true
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node != table && node.Type == html.ElementNode && node.Data == "table" {
			valid = false
			return
		}
		if node.Type == html.ElementNode && node.Data == "tr" {
			var cells []template.HTML
			var headers []string
			for cell := node.FirstChild; cell != nil; cell = cell.NextSibling {
				if cell.Type != html.ElementNode || (cell.Data != "td" && cell.Data != "th") {
					continue
				}
				for _, attr := range cell.Attr {
					if attr.Key == "colspan" || attr.Key == "rowspan" {
						valid = false
					}
				}
				var value bytes.Buffer
				for child := cell.FirstChild; child != nil; child = child.NextSibling {
					if err := html.Render(&value, child); err != nil {
						valid = false
					}
				}
				cells = append(cells, template.HTML(value.String()))
				if cell.Data == "th" {
					headers = append(headers, strings.Join(strings.Fields(tableNodeText(cell)), " "))
				}
			}
			if len(headers) > 0 {
				if len(result.Headers) > 0 || len(headers) != len(cells) || len(result.Rows) > 0 {
					valid = false
				}
				result.Headers = headers
			} else if len(cells) > 0 {
				result.Rows = append(result.Rows, cells)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(table)
	if !valid || len(result.Headers) == 0 {
		return nil, nil
	}
	for _, row := range result.Rows {
		if len(row) != len(result.Headers) {
			return nil, nil
		}
	}
	return result, nil
}

func tableCellText(value template.HTML) string {
	root, _, _, err := reportFragment(value)
	if err != nil {
		return ""
	}
	return strings.Join(strings.Fields(tableNodeText(root)), " ")
}

func tableRowIdentity(table *reportTable, index int) string {
	key := tableCellText(table.Rows[index][0])
	if key == "" {
		return fmt.Sprintf("Row %d", index+1)
	}
	header := table.Headers[0]
	if header == "" {
		header = "Column 1"
	}
	return header + ": " + key
}

func tableCellDisplay(value template.HTML) template.HTML {
	if strings.TrimSpace(string(value)) == "" {
		return `<em class="dyno-change-report-note">Empty</em>`
	}
	return value
}

func tableRowChange(before, after *reportTable, oldIndex, newIndex int) (reportTableRow, error) {
	row := reportTableRow{Kind: "Changed", OldIndex: oldIndex, NewIndex: newIndex}
	table, index := after, newIndex
	if before == nil {
		row.Kind = "Added"
	} else if after == nil {
		table, index = before, oldIndex
		row.Kind = "Removed"
	}
	row.Identity = tableRowIdentity(table, index)
	for column, name := range table.Headers {
		if name == "" {
			name = fmt.Sprintf("Column %d", column+1)
		}
		cell := reportTableCell{Column: name}
		if before != nil {
			cell.Before = before.Rows[oldIndex][column]
		}
		if after != nil {
			cell.After = after.Rows[newIndex][column]
		}
		if before != nil && after != nil {
			if cell.Before == cell.After {
				continue
			}
			old, new, err := highlightReportPair(cell.Before, cell.After)
			if err != nil {
				return row, err
			}
			cell.Before, cell.After = old, new
			cell.MarkupOnly = !strings.Contains(string(old)+string(new), `class="dyno-diff-word `)
		}
		if before != nil {
			cell.Before = tableCellDisplay(cell.Before)
		}
		if after != nil {
			cell.After = tableCellDisplay(cell.After)
		}
		row.Cells = append(row.Cells, cell)
	}
	return row, nil
}

func buildTableReport(before, after template.HTML) (*reportTableDiff, error) {
	if !strings.Contains(string(before)+string(after), "<table") {
		return nil, nil
	}
	left, err := parseReportTable(before)
	if err != nil {
		return nil, err
	}
	right, err := parseReportTable(after)
	if err != nil {
		return nil, err
	}
	if (before != "" && left == nil) || (after != "" && right == nil) {
		return nil, nil
	}
	result := &reportTableDiff{}
	appendRow := func(old, new *reportTable, i, j int) error {
		row, err := tableRowChange(old, new, i, j)
		if err == nil && len(row.Cells) > 0 {
			row.Anchor = fmt.Sprintf("row-%d", len(result.Rows)+1)
			result.Rows = append(result.Rows, row)
		}
		return err
	}
	finish := func() (*reportTableDiff, error) {
		result.Context = reportTableContexts(left, right, result.Rows)
		return result, nil
	}
	if left == nil || right == nil || !slices.Equal(left.Headers, right.Headers) {
		if left != nil && right != nil {
			result.Note = "Column headings changed; removed and added rows are shown in full."
		}
		if left != nil {
			for i := range left.Rows {
				if err := appendRow(left, nil, i, -1); err != nil {
					return nil, err
				}
			}
		}
		if right != nil {
			for j := range right.Rows {
				if err := appendRow(nil, right, -1, j); err != nil {
					return nil, err
				}
			}
		}
		if len(result.Rows) == 0 {
			result.Note = "Table headings: "
			if left != nil {
				result.Note += "Before: " + strings.Join(left.Headers, ", ") + ". "
			}
			if right != nil {
				result.Note += "After: " + strings.Join(right.Headers, ", ") + "."
			}
		}
		return finish()
	}
	counts := func(table *reportTable) map[string]int {
		m := map[string]int{}
		for _, row := range table.Rows {
			m[tableCellText(row[0])]++
		}
		return m
	}
	leftCounts, rightCounts := counts(left), counts(right)
	leftKeys := map[string]int{}
	for i, row := range left.Rows {
		key := tableCellText(row[0])
		if key != "" && leftCounts[key] == 1 && rightCounts[key] == 1 {
			leftKeys[key] = i
		}
	}
	used := make([]bool, len(left.Rows))
	for j, row := range right.Rows {
		i, matched := leftKeys[tableCellText(row[0])]
		if !matched {
			// Duplicate/empty identifiers may still match an exactly unchanged row.
			for candidate, old := range left.Rows {
				if !used[candidate] && slices.Equal(old, row) {
					i, matched = candidate, true
					break
				}
			}
		}
		if matched {
			used[i] = true
			if err := appendRow(left, right, i, j); err != nil {
				return nil, err
			}
		} else if err := appendRow(nil, right, -1, j); err != nil {
			return nil, err
		}
	}
	for i := range left.Rows {
		if !used[i] {
			if err := appendRow(left, nil, i, -1); err != nil {
				return nil, err
			}
		}
	}
	if len(result.Rows) == 0 {
		result.Note = "Only table formatting or row order changed; cell values are unchanged."
	}
	return finish()
}
