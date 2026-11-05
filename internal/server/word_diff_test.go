package server

import (
	"html/template"
	"path/filepath"
	"strings"
	"testing"
)

func TestWordDiffMarksOnlyChangedWordsAndPreservesEscaping(t *testing.T) {
	left, right := "Příliš žlutý release/RE63 <script>", "Příliš modrý release/RE64 <script>"
	removed, added := changedWordRanges(left, right)
	old, new := string(highlightedSource(left, removed, "removed")), string(highlightedSource(right, added, "added"))
	for _, value := range []string{old, new} {
		if strings.Contains(value, "<script>") || !strings.Contains(value, "&lt;script&gt;") || strings.Contains(value, `removed">Příliš`) || strings.Contains(value, `added">Příliš`) {
			t.Fatal(value)
		}
	}
	if !strings.Contains(old, `removed">žlutý</span>`) || !strings.Contains(new, `added">modrý</span>`) || !strings.Contains(new, `added">RE64</span>`) {
		t.Fatalf("unexpected highlighting: %s / %s", old, new)
	}
	if diffVisibleText(t, old) != left || diffVisibleText(t, new) != right {
		t.Fatal("highlighting changed the source")
	}
}

func TestRenderedWordDiffPreservesMarkupAndText(t *testing.T) {
	left := template.HTML(`<p>Keep <strong>old <a href="/same?x=1&amp;y=2">rule</a></strong> &amp; <code>RE63</code>.</p><pre><code class="language-sql"><span>SELECT</span> old;</code></pre>`)
	right := template.HTML(strings.ReplaceAll(strings.ReplaceAll(string(left), "old", "new"), "RE63", "RE64"))
	before, after, err := highlightReportPair(left, right)
	if err != nil {
		t.Fatal(err)
	}
	for i, value := range []template.HTML{before, after} {
		if !strings.Contains(string(value), `<strong>`) || !strings.Contains(string(value), `href="/same?x=1&amp;y=2"`) || !strings.Contains(string(value), `class="language-sql dyno-code-highlight"`) {
			t.Fatal(value)
		}
		original := left
		if i == 1 {
			original = right
		}
		if diffVisibleText(t, string(value)) != diffVisibleText(t, string(original)) {
			t.Fatal("DOM text changed")
		}
	}
	if !strings.Contains(string(after), `added">new</span>`) {
		t.Fatal(after)
	}
}

func TestWordDiffBoundsLargeInputs(t *testing.T) {
	left, right := strings.Repeat("old ", 1100), strings.Repeat("new ", 1100)
	a, b := changedWordRanges(left, right)
	if len(a) != 1 || len(b) != 1 || a[0].end != len(strings.TrimSpace(left)) || b[0].end != len(strings.TrimSpace(right)) {
		t.Fatal("missing bounded fallback")
	}
}

func TestChangeReportOnlyIncludesChangedListItems(t *testing.T) {
	s := newTestServer(t)
	left := "# Guide\n\nIntroductory paragraph without a blank line before the list:\n- Stable first item.\n- Old rule.\n\n  Details remain here.\n\n  - Nested unchanged detail.\n- Stable last item.\n"
	right := strings.Replace(left, "Old rule.", "New rule.", 1)
	changes, added, removed, changed, err := s.buildChangeReport([]byte(left), []byte(right), filepath.Join(s.contentDir, "index.md"))
	if err != nil || len(changes) != 1 || added != 0 || removed != 0 || changed != 1 {
		t.Fatalf("%+v %v", changes, err)
	}
	content := string(changes[0].After)
	if strings.Contains(content, "Stable") || !strings.Contains(content, "Details remain") || !strings.Contains(content, "Nested unchanged") || !strings.Contains(content, `added">New</span>`) {
		t.Fatal(content)
	}
}

func TestChangeReportNotesLinkTargetOnlyChanges(t *testing.T) {
	s := newTestServer(t)
	changes, _, _, _, err := s.buildChangeReport([]byte("Read [guide](/old)."), []byte("Read [guide](/new)."), filepath.Join(s.contentDir, "index.md"))
	if err != nil || len(changes) != 1 || !changes[0].MarkupOnly {
		t.Fatalf("%+v %v", changes, err)
	}
}
