package server

import (
	"fmt"
	"strings"
)

// Show at most twelve identifying rows, with one unchanged neighbor around
// each affected run. Long values remain in the cell details, not this overview.
func reportTableContexts(before, after *reportTable, changes []reportTableRow) []reportTableContext {
	var result []reportTableContext
	for _, side := range []string{"Before", "After"} {
		table := before
		if side == "After" {
			table = after
		}
		if table == nil {
			continue
		}
		affected := map[int]reportTableRow{}
		needed := false
		for _, change := range changes {
			if side == "Before" && change.Kind != "Added" {
				affected[change.OldIndex] = change
				if change.Kind == "Removed" {
					needed = true
				}
			}
			if side == "After" && change.Kind != "Removed" {
				affected[change.NewIndex] = change
				needed = true
			}
		}
		if !needed {
			continue
		}
		context := reportTableContext{Side: side, Column: table.Headers[0], Headers: strings.Join(table.Headers, " | ")}
		if context.Column == "" {
			context.Column = "Column 1"
		}
		count := 0
		for i, row := range table.Rows {
			_, here := affected[i]
			_, previous := affected[i-1]
			_, next := affected[i+1]
			if !(here || previous || next) || count >= 12 {
				if len(context.Rows) == 0 || !context.Rows[len(context.Rows)-1].Gap {
					context.Rows = append(context.Rows, reportTableContextRow{Gap: true})
				}
				continue
			}
			entry := reportTableContextRow{Number: i + 1, Identity: tableCellText(row[0]), Kind: "Unchanged"}
			if entry.Identity == "" {
				entry.Identity = fmt.Sprintf("Row %d", i+1)
			}
			if change, ok := affected[i]; ok {
				entry.Kind, entry.Anchor = change.Kind, change.Anchor
				if change.Kind == "Changed" {
					var columns []string
					for _, cell := range change.Cells {
						columns = append(columns, cell.Column)
					}
					entry.Columns = strings.Join(columns, ", ")
				}
			}
			context.Rows = append(context.Rows, entry)
			count++
		}
		result = append(result, context)
	}
	return result
}
