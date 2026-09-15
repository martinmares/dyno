package tasks

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mares/dyno/internal/navigation"
)

// Item is one markdown task list entry collected from a site.
type Item struct {
	BookSlug   string
	BookTitle  string
	PageTitle  string
	PagePath   string
	PageURL    string
	SourcePath string
	Section    string
	Text       string
	Done       bool
	Order      int
}

// Index holds all task items for one site/book.
type Index struct {
	items    []Item
	pageMeta map[string]PageMeta
}

// HasTasksBlock reports whether the given page contains an embedded ```tasks block.
func (idx *Index) HasTasksBlock(pagePath string) bool {
	if idx == nil {
		return false
	}
	meta, ok := idx.pageMeta[pagePath]
	return ok && meta.HasTasksBlock
}

// PageMeta stores page-level task metadata.
type PageMeta struct {
	HasTasksBlock bool
}

// BuildIndex walks the nav tree and extracts markdown task list entries.
func BuildIndex(bookSlug, bookTitle, contentDir string, nav *navigation.NavNode) (*Index, error) {
	idx := &Index{pageMeta: make(map[string]PageMeta)}
	if nav == nil {
		return idx, nil
	}

	var order int
	var walk func(*navigation.NavNode)
	walk = func(node *navigation.NavNode) {
		if node == nil {
			return
		}
		if node.FSPath != "" && !node.IsDir {
			src, err := os.ReadFile(node.FSPath)
			if err == nil {
				idx.pageMeta[node.FullPath] = PageMeta{
					HasTasksBlock: strings.Contains(string(src), "```tasks"),
				}
				relSource := relativeContentPath(contentDir, filepath.Dir(node.FSPath))
				items := extractItems(string(src), Item{
					BookSlug:   bookSlug,
					BookTitle:  bookTitle,
					PageTitle:  node.Title,
					PagePath:   node.FullPath,
					PageURL:    node.FullPath,
					SourcePath: relSource,
				}, &order)
				idx.items = append(idx.items, items...)
			}
		}
		for _, ch := range node.Children {
			walk(ch)
		}
	}
	walk(nav)
	return idx, nil
}

func relativeContentPath(contentDir, absPath string) string {
	rel, err := filepath.Rel(contentDir, absPath)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

var (
	taskLineRe   = regexp.MustCompile(`^(\s*)[-*+] \[([ xX])\] (.*)$`)
	headingLineR = regexp.MustCompile(`^(#{1,6})\s+(.*\S)\s*$`)
	fenceLineRe  = regexp.MustCompile(`^([` + "`" + `~]{3,})(.*)$`)
)

func extractItems(src string, base Item, order *int) []Item {
	lines := strings.Split(src, "\n")
	items := make([]Item, 0, 8)
	currentSection := base.PageTitle
	fenceMarker := ""
	fenceLen := 0

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")

		if fenceMarker != "" {
			if isClosingFence(trimmed, fenceMarker, fenceLen) {
				fenceMarker = ""
				fenceLen = 0
			}
			continue
		}

		if subs := fenceLineRe.FindStringSubmatch(trimmed); len(subs) == 3 {
			fenceMarker = string(subs[1][0])
			fenceLen = len(subs[1])
			continue
		}

		if subs := headingLineR.FindStringSubmatch(trimmed); len(subs) == 3 {
			currentSection = strings.TrimSpace(subs[2])
			continue
		}

		if subs := taskLineRe.FindStringSubmatch(trimmed); len(subs) == 4 {
			text := strings.TrimSpace(subs[3])
			item := base
			item.Text = text
			item.Done = strings.EqualFold(subs[2], "x")
			item.Section = currentSection
			item.Order = *order
			*order = *order + 1
			items = append(items, item)
		}
	}

	return items
}

func isClosingFence(line, marker string, minLen int) bool {
	trimmed := strings.TrimSpace(line)
	for _, r := range trimmed {
		if r != rune(marker[0]) {
			return false
		}
	}
	return len(trimmed) >= minLen
}

// Render converts a custom tasks block query into HTML.
func (idx *Index) Render(query, anchorID string) (string, error) {
	filter := parseQuery(query)
	items := idx.filter(filter)
	if len(items) == 0 {
		return `<div class="tasks-empty">No matching tasks.</div>`, nil
	}
	return idx.renderGroups(items, anchorID, query, false), nil
}

// RenderSummary renders the whole task collection as a summary page.
func (idx *Index) RenderSummary(anchorID string) string {
	if idx == nil || len(idx.items) == 0 {
		return `<div class="tasks-empty">No tasks were found in this site.</div>`
	}
	return idx.renderGroups(idx.items, anchorID, "", true)
}

// RenderSection renders tasks whose PagePath starts with sectionPrefix (i.e. the current folder).
func (idx *Index) RenderSection(sectionPrefix, anchorID string) string {
	if idx == nil {
		return `<div class="tasks-empty">No tasks were found in this section.</div>`
	}
	var items []Item
	for _, item := range idx.items {
		if strings.HasPrefix(item.PagePath, sectionPrefix) {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return `<div class="tasks-empty">No tasks were found in this section.</div>`
	}
	return idx.renderGroups(items, anchorID, "", false)
}

type sectionGroup struct {
	name  string
	items []Item
}

type pageGroup struct {
	key       string
	title     string
	url       string
	source    string
	hasBlock  bool
	doneCount int
	openCount int
	items     []Item
	sections  []sectionGroup
}

func (idx *Index) renderGroups(items []Item, anchorID, query string, summary bool) string {
	parsed := parseQuery(query)
	pages := map[string]*pageGroup{}
	pageOrder := make([]string, 0)
	for _, item := range items {
		key := item.PageURL
		pg := pages[key]
		if pg == nil {
			meta := idx.pageMeta[key]
			pg = &pageGroup{
				key:      key,
				title:    item.PageTitle,
				url:      item.PageURL,
				source:   item.SourcePath,
				hasBlock: meta.HasTasksBlock,
			}
			pages[key] = pg
			pageOrder = append(pageOrder, key)
		}
		if item.Done {
			pg.doneCount++
		} else {
			pg.openCount++
		}
		pg.items = append(pg.items, item)

		sectionName := item.Section
		if sectionName == "" {
			sectionName = item.PageTitle
		}
		found := false
		for i := range pg.sections {
			if pg.sections[i].name == sectionName {
				pg.sections[i].items = append(pg.sections[i].items, item)
				found = true
				break
			}
		}
		if !found {
			pg.sections = append(pg.sections, sectionGroup{name: sectionName, items: []Item{item}})
		}
	}

	sort.Slice(pageOrder, func(i, j int) bool {
		pi, pj := pages[pageOrder[i]], pages[pageOrder[j]]
		if pi.source == pj.source {
			return pi.title < pj.title
		}
		return pi.source < pj.source
	})

	var b strings.Builder
	b.WriteString(`<div class="tasks-view" data-task-root="1"`)
	if anchorID != "" {
		b.WriteString(` id="`)
		b.WriteString(html.EscapeString(anchorID))
		b.WriteString(`"`)
	}
	b.WriteString(`>`)
	b.WriteString(`<div class="tasks-toolbar"`)
	if anchorID != "" {
		b.WriteString(` id="`)
		b.WriteString(html.EscapeString(anchorID))
		b.WriteString(`-filter"`)
	}
	b.WriteString(`>`)
	b.WriteString(`<div class="tasks-query-summary">`)
	b.WriteString(`<span class="tasks-query-pill tasks-query-pill-total">`)
	b.WriteString(fmt.Sprintf("%d tasks", len(items)))
	b.WriteString(`</span>`)
	b.WriteString(`<span class="tasks-query-pill tasks-query-pill-open">`)
	b.WriteString(fmt.Sprintf("%d open", countOpen(items)))
	b.WriteString(`</span>`)
	b.WriteString(`<span class="tasks-query-pill tasks-query-pill-done">`)
	b.WriteString(fmt.Sprintf("%d done", countDone(items)))
	b.WriteString(`</span>`)
	if query != "" {
		b.WriteString(`<span class="tasks-query-pill tasks-query-pill-total">query</span>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<label class="tasks-filter-wrap">`)
	b.WriteString(`<span class="visually-hidden">Filter tasks</span>`)
	b.WriteString(`<input type="search" class="tasks-filter" placeholder="Filter tasks..." data-task-filter>`)
	b.WriteString(`</label>`)
	b.WriteString(`</div>`)

	if summary {
		b.WriteString(`<div class="tasks-summary-note">Pages with task items and embedded task views.</div>`)
		if len(pageOrder) > 0 {
			b.WriteString(`<div class="tasks-summary-overview">`)
			b.WriteString(`<span class="tasks-summary-overview-label">Pages</span>`)
			b.WriteString(`<div class="tasks-summary-links">`)
			for i, key := range pageOrder {
				pg := pages[key]
				sectionID := fmt.Sprintf("tasks-page-%d", i)
				b.WriteString(`<a class="tasks-summary-chip" href="#`)
				b.WriteString(sectionID)
				b.WriteString(`">`)
				b.WriteString(html.EscapeString(pg.title))
				b.WriteString(`<span>`)
				b.WriteString(fmt.Sprintf("%d open / %d done", pg.openCount, pg.doneCount))
				b.WriteString(`</span></a>`)
			}
			b.WriteString(`</div></div>`)
		}
	}

	if len(pageOrder) == 0 {
		b.WriteString(`<div class="tasks-empty" data-task-empty>No matching tasks.</div>`)
	} else {
		b.WriteString(`<div class="tasks-empty" data-task-empty hidden>No matching tasks.</div>`)
	}

	for i, key := range pageOrder {
		pg := pages[key]
		link := pg.url
		if pg.hasBlock {
			link = pg.url + "#tasks-0"
		}
		sectionID := fmt.Sprintf("tasks-page-%d", i)
		b.WriteString(`<section class="tasks-page" data-task-page-group="1" id="`)
		b.WriteString(sectionID)
		b.WriteString(`">`)
		b.WriteString(`<header class="tasks-page-header">`)
		b.WriteString(`<div class="tasks-page-heading">`)
		b.WriteString(`<a class="tasks-page-title" href="`)
		b.WriteString(html.EscapeString(link))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(pg.title))
		b.WriteString(`</a>`)
		b.WriteString(`<span class="tasks-page-source">`)
		b.WriteString(html.EscapeString(pg.source))
		b.WriteString(`</span>`)
		b.WriteString(`</div>`)
		b.WriteString(`<div class="tasks-page-meta">`)
		b.WriteString(`<span class="tasks-page-counts">`)
		b.WriteString(fmt.Sprintf("%d open / %d done", pg.openCount, pg.doneCount))
		b.WriteString(`</span>`)
		b.WriteString(`</div>`)
		b.WriteString(`</header>`)

		if parsed.SortByDescription {
			for i := range pg.sections {
				sort.SliceStable(pg.sections[i].items, func(a, b int) bool {
					ia := strings.ToLower(pg.sections[i].items[a].Text)
					ib := strings.ToLower(pg.sections[i].items[b].Text)
					if ia == ib {
						return pg.sections[i].items[a].Order < pg.sections[i].items[b].Order
					}
					return ia < ib
				})
			}
		}

		for _, section := range pg.sections {
			if section.name != "" && section.name != pg.title {
				b.WriteString(`<div class="tasks-section-title">`)
				b.WriteString(html.EscapeString(section.name))
				b.WriteString(`</div>`)
			}
			b.WriteString(renderTaskRows(section.items))
		}

		b.WriteString(`</section>`)
	}

	b.WriteString(`</div>`)
	return b.String()
}

func renderTaskRows(items []Item) string {
	var b strings.Builder
	b.WriteString(`<div class="tasks-table" role="table">`)
	b.WriteString(`<div class="tasks-table-head" role="row">`)
	b.WriteString(`<span>#</span><span>Task</span><span>Status</span>`)
	b.WriteString(`</div>`)
	for i, item := range items {
		stateClass := "tasks-row-open"
		state := "Open"
		checked := ""
		if item.Done {
			stateClass = "tasks-row-done"
			state = "Done"
			checked = " checked"
		}
		searchText := strings.ToLower(item.PageTitle + " " + item.Section + " " + item.SourcePath + " " + item.Text)
		b.WriteString(`<div class="tasks-row `)
		b.WriteString(stateClass)
		b.WriteString(`" role="row" data-task-row="1" data-task-search="`)
		b.WriteString(html.EscapeString(searchText))
		b.WriteString(`">`)
		b.WriteString(`<span class="tasks-row-index">`)
		b.WriteString(fmt.Sprintf("%d", i+1))
		b.WriteString(`</span>`)
		b.WriteString(`<label class="tasks-row-task">`)
		b.WriteString(`<input type="checkbox" disabled`)
		b.WriteString(checked)
		b.WriteString(`>`)
		b.WriteString(`<span class="tasks-item-text">`)
		b.WriteString(renderInlineText(item.Text))
		b.WriteString(`</span>`)
		b.WriteString(`</label>`)
		b.WriteString(`<span class="tasks-row-state">`)
		b.WriteString(state)
		b.WriteString(`</span>`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func (idx *Index) filter(filter Query) []Item {
	if idx == nil || len(idx.items) == 0 {
		return nil
	}
	out := make([]Item, 0, len(idx.items))
	for _, item := range idx.items {
		if !filter.Matches(item) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func countDone(items []Item) int {
	n := 0
	for _, item := range items {
		if item.Done {
			n++
		}
	}
	return n
}

func countOpen(items []Item) int {
	return len(items) - countDone(items)
}

// Query filters the tasks block.
type Query struct {
	Status            string
	PathIncludes      string
	SortByDescription bool
}

func parseQuery(query string) Query {
	var q Query
	for _, line := range strings.Split(query, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case line == "done":
			q.Status = "done"
		case line == "not done":
			q.Status = "not-done"
		case strings.HasPrefix(line, "path includes "):
			q.PathIncludes = strings.TrimSpace(strings.TrimPrefix(line, "path includes "))
		case strings.HasPrefix(line, "sort by "):
			if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "sort by ")), "description") {
				q.SortByDescription = true
			}
		}
	}
	return q
}

func (q Query) Matches(item Item) bool {
	switch q.Status {
	case "done":
		if !item.Done {
			return false
		}
	case "not-done":
		if item.Done {
			return false
		}
	}
	if q.PathIncludes != "" {
		if !strings.Contains(item.SourcePath, q.PathIncludes) && !strings.Contains(item.PagePath, q.PathIncludes) {
			return false
		}
	}
	return true
}

func renderInlineText(text string) string {
	if text == "" {
		return ""
	}
	parts := strings.Split(text, "`")
	var b strings.Builder
	for i, part := range parts {
		if i%2 == 1 {
			b.WriteString(`<code>`)
			b.WriteString(html.EscapeString(part))
			b.WriteString(`</code>`)
			continue
		}
		b.WriteString(html.EscapeString(part))
	}
	return b.String()
}
