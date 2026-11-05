package server

import (
	"html/template"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/markdown"
)

func TestReportTableWithForcedInteractiveTables(t *testing.T) {
	s := newTestServer(t)
	renderer, err := markdown.NewRendererWithOptions(markdown.RendererOptions{InteractiveTables: true})
	if err != nil {
		t.Fatal(err)
	}
	s.renderer = renderer
	left := "| Repository | Description |\n| --- | --- |\n| unchanged | Unrelated content. |\n| app | Old rule. |\n"
	right := strings.Replace(left, "Old rule.", "New rule.", 1)
	changes, _, _, _, err := s.buildChangeReport([]byte(left), []byte(right), filepath.Join(s.contentDir, "index.md"))
	if err != nil || len(changes) != 1 || changes[0].Table == nil || len(changes[0].Table.Rows) != 1 {
		t.Fatalf("interactive tables bypassed cell comparison: %+v %v", changes, err)
	}
	rec := httptest.NewRecorder()
	if err := s.render(rec, "git-compare.html", gitCompareData{Mode: "report", Changes: changes}); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<table") || strings.Contains(body, "dyno-table-widget") || strings.Contains(body, "Unrelated content") || !strings.Contains(body, "Repository: app") || !strings.Contains(body, `added">New</span>`) {
		t.Fatal("interactive wrapper or unchanged content leaked into report")
	}
	// The same renderer must still retain interactive tables outside the report.
	preview, _, err := s.renderComparedMarkdown([]byte(left), []byte(right))
	if err != nil || !strings.Contains(string(preview), "dyno-table-widget") {
		t.Fatal("Preview lost its interactive table")
	}
}

func TestReportTableShowsOnlyChangedCells(t *testing.T) {
	s := newTestServer(t)
	left := "# Guide\n\n| Repository | Team | Description |\n| --- | --- | --- |\n| stable | Platform | Do not repeat this row. |\n| `app` | Platform | Use the old [rule](/guide). |\n"
	right := strings.Replace(left, "old", "new", 1)
	changes, added, removed, changed, err := s.buildChangeReport([]byte(left), []byte(right), filepath.Join(s.contentDir, "index.md"))
	if err != nil || len(changes) != 1 || added != 0 || removed != 0 || changed != 1 {
		t.Fatalf("%+v %v", changes, err)
	}
	table := changes[0].Table
	if table == nil || len(table.Rows) != 1 || table.Rows[0].Identity != "Repository: app" || len(table.Rows[0].Cells) != 1 {
		t.Fatalf("unexpected table: %+v", table)
	}
	cell := table.Rows[0].Cells[0]
	if cell.Column != "Description" || !strings.Contains(string(cell.After), `added">new</span>`) || !strings.Contains(string(cell.After), `href="/guide"`) {
		t.Fatalf("cell markup lost: %+v", cell)
	}
	rec := httptest.NewRecorder()
	if err := s.render(rec, "report-values", cell); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), "<table") || !strings.Contains(rec.Body.String(), "Before") || !strings.Contains(rec.Body.String(), "After") {
		t.Fatal(rec.Body.String())
	}
	rec = httptest.NewRecorder()
	if err := s.render(rec, "git-compare.html", gitCompareData{Mode: "report", Changes: changes}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), "<table") || strings.Contains(rec.Body.String(), "Do not repeat this row") || strings.Contains(rec.Body.String(), ">Platform<") || !strings.Contains(rec.Body.String(), "Repository: app") || !strings.Contains(rec.Body.String(), "Description") {
		t.Fatal("full tables or unchanged values leaked into rendered report")
	}
}

func TestReportTableAddedRemovedAndEmptyValues(t *testing.T) {
	before := template.HTML(`<table><thead><tr><th>ID</th><th>Value</th></tr></thead><tbody><tr><td>old</td><td>Gone</td></tr><tr><td>same</td><td></td></tr></tbody></table>`)
	after := template.HTML(`<table><thead><tr><th>ID</th><th>Value</th></tr></thead><tbody><tr><td>new</td><td>Added</td></tr><tr><td>same</td><td>Filled</td></tr></tbody></table>`)
	diff, err := buildTableReport(before, after)
	if err != nil || diff == nil || len(diff.Rows) != 3 {
		t.Fatalf("%+v %v", diff, err)
	}
	if diff.Rows[0].Kind != "Added" || len(diff.Rows[0].Cells) != 2 || diff.Rows[1].Kind != "Changed" || diff.Rows[2].Kind != "Removed" || len(diff.Rows[2].Cells) != 2 {
		t.Fatalf("%+v", diff.Rows)
	}
	if !strings.Contains(string(diff.Rows[1].Cells[0].Before), "Empty") {
		t.Fatal("empty original value is not identified")
	}
	for _, pair := range [][2]template.HTML{{"", after}, {before, ""}} {
		diff, err := buildTableReport(pair[0], pair[1])
		if err != nil || len(diff.Rows) != 2 {
			t.Fatalf("%+v %v", diff, err)
		}
	}
}

func TestReportTableConservativeMatchingAndStructureChanges(t *testing.T) {
	table := func(header, rows string) template.HTML {
		return template.HTML(`<table><thead><tr><th>ID</th><th>` + header + `</th></tr></thead><tbody>` + rows + `</tbody></table>`)
	}
	left := table("Value", `<tr><td>duplicate</td><td>Stable</td></tr><tr><td>duplicate</td><td>Old</td></tr>`)
	right := table("Value", `<tr><td>duplicate</td><td>Stable</td></tr><tr><td>duplicate</td><td>New</td></tr>`)
	diff, err := buildTableReport(left, right)
	if err != nil || len(diff.Rows) != 2 || diff.Rows[0].Kind != "Added" || diff.Rows[1].Kind != "Removed" {
		t.Fatalf("ambiguous identifiers must not be paired: %+v %v", diff, err)
	}
	diff, err = buildTableReport(left, table("Renamed", `<tr><td>duplicate</td><td>Stable</td></tr>`))
	if err != nil || diff.Note == "" || len(diff.Rows) != 3 {
		t.Fatalf("header changes missing: %+v %v", diff, err)
	}
	diff, err = buildTableReport(table("Value", `<tr><td>A</td><td>1</td></tr><tr><td>B</td><td>2</td></tr>`), table("Value", `<tr><td>B</td><td>2</td></tr><tr><td>A</td><td>1</td></tr>`))
	if err != nil || len(diff.Rows) != 0 || diff.Note == "" {
		t.Fatalf("reordering must be explained: %+v %v", diff, err)
	}
	diff, err = buildTableReport(template.HTML(`<table><tr><th>ID</th></tr><tr><td colspan="2">Unsupported</td></tr></table>`), "")
	if err != nil || diff != nil {
		t.Fatal("complex HTML tables must retain the ordinary report fallback")
	}
}

func TestReportTableMarkupOnlyCellChangeAndBlankHeaders(t *testing.T) {
	left := template.HTML(`<table><thead><tr><th></th><th></th></tr></thead><tbody><tr><td>key</td><td><a href="/old">Guide</a></td></tr></tbody></table>`)
	right := template.HTML(strings.Replace(string(left), "/old", "/new", 1))
	diff, err := buildTableReport(left, right)
	if err != nil || len(diff.Rows) != 1 || diff.Rows[0].Identity != "Column 1: key" || diff.Rows[0].Cells[0].Column != "Column 2" || !diff.Rows[0].Cells[0].MarkupOnly {
		t.Fatalf("%+v %v", diff, err)
	}
}
