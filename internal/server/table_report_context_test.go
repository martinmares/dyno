package server

import (
	"fmt"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTableContextShowsNeighborsAndLinksToDetails(t *testing.T) {
	makeTable := func(rows string) template.HTML {
		return template.HTML(`<table><thead><tr><th>ID</th><th>Description</th></tr></thead><tbody>` + rows + `</tbody></table>`)
	}
	row := func(id, value string) string { return `<tr><td>` + id + `</td><td>` + value + `</td></tr>` }
	left := makeTable(row("A", "A long unrelated value") + row("B", "Another hidden value") + row("C", "Old description") + row("D", "Stable") + row("E", "Stable"))
	right := makeTable(row("A", "A long unrelated value") + row("B", "Another hidden value") + row("C", "New description") + row("X", "Inserted value") + row("D", "Stable") + row("E", "Stable"))
	diff, err := buildTableReport(left, right)
	if err != nil || len(diff.Context) != 1 || diff.Context[0].Side != "After" {
		t.Fatalf("%+v %v", diff, err)
	}
	context := diff.Context[0]
	var identifiers []string
	for _, entry := range context.Rows {
		if entry.Gap {
			continue
		}
		identifiers = append(identifiers, entry.Identity)
		if entry.Identity == "C" && (entry.Kind != "Changed" || entry.Columns != "Description" || entry.Anchor == "" || entry.Number != 3) {
			t.Fatalf("wrong changed context row: %+v", entry)
		}
		if entry.Identity == "X" && (entry.Kind != "Added" || entry.Number != 4 || entry.Anchor == "") {
			t.Fatalf("wrong inserted position: %+v", entry)
		}
	}
	if strings.Join(identifiers, ",") != "B,C,X,D" {
		t.Fatalf("missing immediate neighbors: %+v", context.Rows)
	}
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	if err := s.render(rec, "git-compare.html", gitCompareData{Mode: "report", Changes: []documentChange{{ID: "change-7", Section: "Tables", Kind: "Changed", Table: diff}}}); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="#change-7-row-1"`) || !strings.Contains(body, `id="change-7-row-1"`) || strings.Contains(body, "A long unrelated value") || strings.Contains(body, "Another hidden value") {
		t.Fatal("context links are broken or full cell values leaked")
	}
	removed, err := buildTableReport(left, makeTable(row("A", "A long unrelated value")+row("B", "Another hidden value")+row("D", "Stable")+row("E", "Stable")))
	if err != nil || len(removed.Context) != 1 || removed.Context[0].Side != "Before" {
		t.Fatalf("removed row lacks original context: %+v %v", removed, err)
	}
}

func TestTableContextIsBoundedForWholeAddedTables(t *testing.T) {
	var rows strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&rows, "<tr><td>item-%d</td><td>value</td></tr>", i)
	}
	diff, err := buildTableReport("", template.HTML("<table><thead><tr><th>ID</th><th>Value</th></tr></thead><tbody>"+rows.String()+"</tbody></table>"))
	if err != nil || len(diff.Context) != 1 {
		t.Fatalf("%+v %v", diff, err)
	}
	context := diff.Context[0]
	count := 0
	gap := false
	for _, entry := range context.Rows {
		if entry.Gap {
			gap = true
		} else {
			count++
		}
	}
	if count != 12 || !gap || len(diff.Rows) != 40 {
		t.Fatalf("context must be bounded without removing detail: %d %+v", count, context)
	}
}
