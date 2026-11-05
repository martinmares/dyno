package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReportGroupsKeepOneSectionAndSeparateChangeKinds(t *testing.T) {
	changes := []documentChange{{ID: "change-1", Section: "Synchronization", Kind: "Added", BlockType: "Text", After: "New instruction"}, {ID: "change-2", Section: "Other", Kind: "Changed"}, {ID: "change-3", Section: "Synchronization", Kind: "Removed", BlockType: "List", Before: "Old instruction"}, {ID: "change-4", Section: "Synchronization", Kind: "Changed", Before: "Before", After: "After"}}
	groups := groupReportChanges(changes)
	if len(groups) != 2 || groups[0].Added != 1 || groups[0].Removed != 1 || groups[0].Changed != 1 || groups[0].Section != "Synchronization" {
		t.Fatalf("%+v", groups)
	}
	for i, id := range []string{"change-4", "change-3", "change-1"} {
		if groups[0].Sets[i].Changes[0].ID != id {
			t.Fatal("grouped content order/anchors lost")
		}
	}
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	if err := s.render(rec, "git-compare.html", gitCompareData{Mode: "report", Changes: changes}); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if strings.Count(body, "<h2>Synchronization</h2>") != 1 || !strings.Contains(body, `href="#report-section-1"`) || !strings.Contains(body, "Removed parts") || !strings.Contains(body, "Added parts") || !strings.Contains(body, `id="change-3"`) {
		t.Fatal("section grouping is missing or hides block anchors")
	}
}
