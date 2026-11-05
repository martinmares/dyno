package server

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReportListsDoNotPairUnrelatedBulletsAndNumberedSteps(t *testing.T) {
	s := newTestServer(t)
	left := "# Guide\n\n- Obsah vznikne jen ve zdrojovém repozitáři. Párový repozitář bude založen prázdný.\n- UI: zdroje doménových knihoven budou převedeny z produktového monorepa do Datalite repozitářů.\n"
	right := "# Guide\n\n3. Schedulovaná pipeline v Datalite načte Release Tag a commit s potřebnou historií.\n4. Pro opravu v interní vývojové historii se původní commit dohledá ze zprávy souhrnného commitu.\n"
	changes, added, removed, changed, err := s.buildChangeReport([]byte(left), []byte(right), filepath.Join(s.contentDir, "index.md"))
	if err != nil || added != 2 || removed != 2 || changed != 0 || len(changes) != 4 {
		t.Fatalf("unrelated items paired: +%d -%d ~%d %+v %v", added, removed, changed, changes, err)
	}
	for _, change := range changes {
		if change.Before != "" && change.After != "" {
			t.Fatal("unrelated before/after pair")
		}
	}
}

func TestReportListsAlignSimilarItemsAfterAnInsertion(t *testing.T) {
	s := newTestServer(t)
	left := "# Guide\n\n- Build application images.\n- Validate database permissions.\n"
	right := "# Guide\n\n1. Read completely unrelated release notes.\n2. Build application packages.\n3. Validate database privileges.\n"
	changes, added, removed, changed, err := s.buildChangeReport([]byte(left), []byte(right), filepath.Join(s.contentDir, "index.md"))
	if err != nil || added != 1 || removed != 0 || changed != 2 {
		t.Fatalf("inserted step shifted matches: +%d -%d ~%d %+v %v", added, removed, changed, changes, err)
	}
	for _, change := range changes {
		if change.Kind != "Changed" {
			continue
		}
		before, after := diffVisibleText(t, string(change.Before)), diffVisibleText(t, string(change.After))
		if strings.Contains(before, "Build") != strings.Contains(after, "Build") || strings.Contains(after, "unrelated") {
			t.Fatalf("mismatched items: %+v", change)
		}
	}
}

func TestReportListMarkerOnlyChangesStillPair(t *testing.T) {
	s := newTestServer(t)
	changes, added, removed, changed, err := s.buildChangeReport([]byte("- Build the application.\n"), []byte("4. Build the application.\n"), filepath.Join(s.contentDir, "index.md"))
	if err != nil || added != 0 || removed != 0 || changed != 1 || len(changes) != 1 || !changes[0].MarkupOnly {
		t.Fatalf("list format-only edit lost: %+v %v", changes, err)
	}
	if !strings.Contains(string(changes[0].Before), "<ul>") || !strings.Contains(string(changes[0].After), `start="4"`) {
		t.Fatal("original numbering was not preserved")
	}
}
