package server

import (
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeReportKeepsChangedLinesAndContextWithFullDisclosure(t *testing.T) {
	s := newTestServer(t)
	var code strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&code, "line-%02d: Příliš <script> old value\n", i)
	}
	left := "# Guide\n\n```text\n" + code.String() + "```\n"
	right := strings.Replace(left, "line-10: Příliš <script> old value", "line-10: Příliš <script> new value", 1)
	changes, _, _, _, err := s.buildChangeReport([]byte(left), []byte(right), filepath.Join(s.contentDir, "index.md"))
	if err != nil || len(changes) != 1 || changes[0].Code == nil {
		t.Fatalf("missing compact code report: %+v %v", changes, err)
	}
	preview := changes[0].Code
	if len(preview.Hunks) != 1 || preview.Hunks[0].Location != "Before lines 9-13; After lines 9-13" {
		t.Fatalf("wrong excerpt boundaries: %+v", preview)
	}
	hunk := preview.Hunks[0]
	text := diffVisibleText(t, string(hunk.After))
	if !strings.Contains(text, "line-08") || !strings.Contains(text, "line-12") || strings.Contains(text, "line-07") || strings.Contains(text, "line-13") || !strings.Contains(text, "Příliš <script> new value") {
		t.Fatalf("wrong excerpt text: %s", text)
	}
	if !strings.Contains(string(hunk.After), `added">new</span>`) || strings.Contains(string(hunk.After), "<script>") {
		t.Fatal("word markup lost or literal code not escaped")
	}
	if !strings.Contains(string(changes[0].Before), "line-00") || !strings.Contains(string(changes[0].After), "line-19") {
		t.Fatal("full code was removed")
	}
	rec := httptest.NewRecorder()
	if err := s.render(rec, "git-compare.html", gitCompareData{Mode: "report", Changes: changes}); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Show complete code block") || !strings.Contains(body, "data-report-code-full") || !strings.Contains(body, "Before lines 9-13") {
		t.Fatal("missing full code disclosure or line location")
	}
}

func TestCodeReportMergesOverlappingContextAndKeepsSeparateHunks(t *testing.T) {
	s := newTestServer(t)
	var code strings.Builder
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&code, "line-%02d old\n", i)
	}
	left := "```sql\n" + code.String() + "```\n"
	right := strings.NewReplacer("line-05 old", "line-05 new", "line-08 old", "line-08 new", "line-24 old", "line-24 new").Replace(left)
	changes, _, _, _, err := s.buildChangeReport([]byte(left), []byte(right), filepath.Join(s.contentDir, "index.md"))
	if err != nil || len(changes) != 1 || changes[0].Code == nil || len(changes[0].Code.Hunks) != 2 {
		t.Fatalf("%+v %v", changes, err)
	}
	if !strings.Contains(changes[0].Code.Hunks[0].Location, "4-11") || !strings.Contains(changes[0].Code.Hunks[1].Location, "23-27") {
		t.Fatal("overlapping context was not merged correctly")
	}
}

func TestSmallAndEntirelyAddedCodeBlocksRemainWhole(t *testing.T) {
	s := newTestServer(t)
	for _, pair := range [][2]string{{"```sql\nSELECT 1;\n```", "```sql\nSELECT 2;\n```"}, {"", "```text\nNew code\n```"}} {
		changes, _, _, _, err := s.buildChangeReport([]byte(pair[0]), []byte(pair[1]), filepath.Join(s.contentDir, "index.md"))
		if err != nil || len(changes) != 1 || changes[0].Code != nil {
			t.Fatalf("short/added code should not be truncated: %+v %v", changes, err)
		}
	}
}
