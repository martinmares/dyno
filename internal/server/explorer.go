package server

import (
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mares/dyno/internal/frontmatter"
	"github.com/mares/dyno/internal/metadata"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

type explorerDocument struct {
	URL, Title, Path, Folder, Summary string
	Headings                          []string
	Values                            map[string]string
	Modified                          time.Time
	Incoming, Outgoing                []navigation.Backlink
}

type explorerCatalog struct {
	Documents map[string]*explorerDocument
	Index     *search.Index
	Metadata  *metadata.Index
}

type explorerLink struct {
	Label, URL string
	Count      int
}
type explorerRow struct {
	Document *explorerDocument
	URL      string
	Values   []string
	Selected bool
}
type explorerColumn struct {
	Name, Label, Type string
	Selected          bool
}
type explorerFolder struct {
	Name, URL      string
	Count          int
	Open, Selected bool
	Children       []*explorerFolder
}
type explorerEdge struct {
	From, To       *explorerDocument
	FromURL, ToURL string
}
type explorerData struct {
	PageData
	ExplorerJSURL, ExplorerCSSURL                          string
	State                                                  string
	DateField, After, Before, MissingField                 string
	DateFields                                             []explorerColumn
	OnlyOrphans                                            bool
	Query, Folder, Scope, Match, Sort, Order, Tab          string
	Columns, AvailableColumns                              []explorerColumn
	Rows                                                   []explorerRow
	Folders                                                []*explorerFolder
	Facets                                                 []metadata.Facet
	Selection                                              *explorerDocument
	SelectionURL                                           string
	SelectionValues                                        []MetadataFieldData
	Incoming, Outgoing                                     []explorerLink
	Total, AllTotal, Eligible, Page, Pages, Orphans, Links int
	PreviousURL, NextURL, AllURL                           string
	Tabs                                                   []explorerLink
	FolderCounts, Missing, Connected, Recent, Groups       []explorerLink
	Edges                                                  []explorerEdge
}

// The catalog is immutable between reloads or editor saves. Build under the
// server lock so an edit cannot publish a stale snapshot after invalidation.
func (s *Server) getExplorerCatalog() (*explorerCatalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.explorerCatalog != nil {
		return s.explorerCatalog, nil
	}
	idx, err := search.BuildIndex(s.nav, func(src string) (string, error) { return s.renderer.ToPlainText([]byte(src)) })
	if err != nil {
		return nil, err
	}
	meta, err := metadata.Build(s.nav, s.siteCfg.Frontmatter)
	if err != nil {
		return nil, err
	}
	backlinks := navigation.BuildBacklinks(s.nav, s.basePath)
	catalog := &explorerCatalog{Documents: make(map[string]*explorerDocument), Index: idx, Metadata: meta}
	indexed := make(map[string]search.Document)
	for _, doc := range idx.Documents() {
		indexed[doc.Path] = doc
	}
	var buildErr error
	navigation.WalkNodes(s.nav, func(node *navigation.NavNode) {
		if buildErr != nil || node.FSPath == "" {
			return
		}
		indexedDoc, ok := indexed[node.FullPath]
		if !ok {
			return
		}
		src, err := s.readDocument(node.FSPath)
		if err != nil {
			buildErr = err
			return
		}
		values, err := frontmatter.Read(src)
		if err != nil {
			buildErr = err
			return
		}
		rel, err := filepath.Rel(s.contentDir, node.FSPath)
		if err != nil {
			buildErr = err
			return
		}
		info, err := s.documentStat(node.FSPath)
		if err != nil {
			buildErr = err
			return
		}
		doc := &explorerDocument{URL: node.FullPath, Title: node.Title, Path: filepath.ToSlash(rel), Values: make(map[string]string), Modified: info.ModTime(), Headings: indexedDoc.Headings}
		if title, ok := values["title"].(string); ok && strings.TrimSpace(title) != "" {
			doc.Title = title
		}
		doc.Folder = path.Dir(doc.Path)
		if doc.Folder == "." {
			doc.Folder = ""
		}
		for _, field := range s.siteCfg.Frontmatter.OrderedFields() {
			doc.Values[field.Name], _ = metadataFieldValue(values[field.Name], "tags")
		}
		for _, line := range strings.Split(indexedDoc.Body, "\n") {
			line = strings.TrimSpace(line)
			if utf8.RuneCountInString(line) > 40 && line != doc.Title {
				runes := []rune(line)
				if len(runes) > 320 {
					line = string(runes[:320]) + "..."
				}
				doc.Summary = line
				break
			}
		}
		catalog.Documents[doc.URL] = doc
	})
	if buildErr != nil {
		return nil, buildErr
	}
	for target, sources := range backlinks {
		to := catalog.Documents[target]
		if to == nil {
			continue
		}
		for _, source := range sources {
			from := catalog.Documents[source.FullPath]
			if from == nil {
				continue
			}
			to.Incoming = append(to.Incoming, navigation.Backlink{Title: from.Title, FullPath: from.URL})
			from.Outgoing = append(from.Outgoing, navigation.Backlink{Title: to.Title, FullPath: to.URL})
		}
	}
	for _, doc := range catalog.Documents {
		for _, links := range [][]navigation.Backlink{doc.Incoming, doc.Outgoing} {
			sort.Slice(links, func(i, j int) bool { return links[i].FullPath < links[j].FullPath })
		}
	}
	s.idx, s.metadata, s.backlinks, s.explorerCatalog = idx, meta, backlinks, catalog
	return catalog, nil
}

func explorerChoice(value, fallback string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}

func cloneExplorerQuery(values url.Values) url.Values {
	copy := make(url.Values, len(values))
	for key, items := range values {
		copy[key] = append([]string(nil), items...)
	}
	return copy
}

func (s *Server) explorerReturnURL(state string) string {
	if !s.explore {
		return ""
	}
	base := s.basePath + "/_explore"
	if values, err := url.ParseQuery(state); err == nil && len(values) > 0 {
		return base + "?" + values.Encode()
	}
	return base
}

func (s *Server) explorerHandler(w http.ResponseWriter, r *http.Request) {
	if isHTMX(r) {
		// Explorer has its own layout and must be opened with a full navigation.
		w.Header().Set("HX-Redirect", r.URL.RequestURI())
		w.WriteHeader(http.StatusOK)
		return
	}
	catalog, err := s.getExplorerCatalog()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	q := r.URL.Query()
	if catalog.Documents[q.Get("selected")] == nil {
		q.Del("selected")
	}
	q.Set("tab", explorerChoice(q.Get("tab"), "documents", "documents", "overview", "relations"))
	q.Set("scope", explorerChoice(q.Get("scope"), "all", "all", "title", "path", "body"))
	q.Set("match", explorerChoice(q.Get("match"), "all", "all", "any"))
	q.Set("order", explorerChoice(q.Get("order"), "asc", "asc", "desc"))
	folder := strings.Trim(q.Get("folder"), "/")
	q.Set("folder", folder)
	base := s.basePath + "/_explore"
	link := func(changes ...string) string {
		copy := cloneExplorerQuery(q)
		for i := 0; i+1 < len(changes); i += 2 {
			if changes[i+1] == "" {
				copy.Del(changes[i])
			} else {
				copy.Set(changes[i], changes[i+1])
			}
		}
		return base + "?" + copy.Encode()
	}
	data := explorerData{PageData: PageData{Title: "Explorer", Site: s.siteCfg, BasePath: s.basePath, BulmaCSSURL: s.assetURL("bulma.min.css"), AppCSSURL: s.assetURL("app.css"), AppJSURL: s.assetURL("app.js"), FaviconURL: s.faviconURL(), BuildVersion: s.version}, Query: q.Get("q"), Folder: folder, Scope: q.Get("scope"), Match: q.Get("match"), Tab: q.Get("tab"), Order: q.Get("order"), AllTotal: len(catalog.Documents)}
	fields := s.siteCfg.Frontmatter.OrderedFields()
	data.ExplorerJSURL = s.assetURL("explorer.js")
	data.ExplorerCSSURL = s.assetURL("explorer.css")
	selectedColumns := make(map[string]bool)
	for _, name := range q["columns"] {
		selectedColumns[name] = true
	}
	if q.Get("columns_set") == "" && len(selectedColumns) == 0 {
		for i, field := range fields {
			if i < 3 {
				selectedColumns[field.Name] = true
			}
		}
	}
	q.Del("columns")
	validSort := []string{"relevance", "title", "path", "modified"}
	for _, field := range fields {
		label := field.Label
		if label == "" {
			label = field.Name
		}
		column := explorerColumn{Name: field.Name, Label: label, Type: field.Type, Selected: selectedColumns[field.Name]}
		data.AvailableColumns = append(data.AvailableColumns, column)
		if column.Selected {
			data.Columns = append(data.Columns, column)
			q.Add("columns", field.Name)
		}
		validSort = append(validSort, "field:"+field.Name)
	}
	q.Set("columns_set", "1")
	data.MissingField = q.Get("missing")
	if _, ok := s.siteCfg.Frontmatter.Fields[data.MissingField]; !ok {
		data.MissingField = ""
		q.Del("missing")
	}
	for _, field := range data.AvailableColumns {
		if field.Type == "date" || field.Type == "datetime" {
			data.DateFields = append(data.DateFields, field)
		}
	}
	data.DateField = q.Get("date_field")
	dateType := s.siteCfg.Frontmatter.Fields[data.DateField].Type
	if dateType != "date" && dateType != "datetime" {
		data.DateField = ""
		q.Del("date_field")
	}
	data.After, data.Before = q.Get("after"), q.Get("before")
	for _, bound := range []string{data.After, data.Before} {
		if bound != "" {
			if _, err := time.Parse("2006-01-02", bound); err != nil {
				http.Error(w, "Date bounds must use YYYY-MM-DD.", http.StatusBadRequest)
				return
			}
		}
	}
	if data.After != "" && data.Before != "" && data.After > data.Before {
		http.Error(w, "The start date must not follow the end date.", http.StatusBadRequest)
		return
	}
	data.OnlyOrphans = q.Get("orphans") == "1"
	defaultSort := "path"
	if strings.TrimSpace(data.Query) != "" {
		defaultSort = "relevance"
	}
	data.Sort = explorerChoice(q.Get("sort"), defaultSort, validSort...)
	q.Set("sort", data.Sort)
	filter := catalog.Metadata.Parse(q)
	allowed := catalog.Metadata.Allowed(filter)
	results := catalog.Index.Query(data.Query, search.Options{Match: data.Match, Scope: data.Scope, OmitSnippets: true}, nil)
	queryAllowed := make(map[string]bool)
	var eligible, docs []*explorerDocument
	scores := make(map[string]float64)
	for _, result := range results {
		doc := catalog.Documents[result.Path]
		if doc == nil {
			continue
		}
		if data.MissingField != "" && strings.TrimSpace(doc.Values[data.MissingField]) != "" {
			continue
		}
		if data.OnlyOrphans && len(doc.Incoming) > 0 {
			continue
		}
		if data.DateField != "" && (data.After != "" || data.Before != "") {
			value := doc.Values[data.DateField]
			if len(value) < 10 {
				continue
			}
			date := value[:10]
			if _, err := time.Parse("2006-01-02", date); err != nil {
				continue
			}
			if data.After != "" && date < data.After || data.Before != "" && date > data.Before {
				continue
			}
		}
		inFolder := folder == "" || doc.Folder == folder || strings.HasPrefix(doc.Folder, folder+"/")
		if inFolder {
			queryAllowed[doc.URL] = true
		}
		if allowed != nil && !allowed[doc.URL] {
			continue
		}
		eligible = append(eligible, doc)
		if inFolder {
			docs = append(docs, doc)
			scores[doc.URL] = result.Score
		}
	}
	data.Facets = catalog.Metadata.FacetsFiltered(filter, queryAllowed)
	data.Eligible = len(eligible)
	data.Total = len(docs)
	data.Folders = explorerFolders(eligible, folder, func(folder string) string { return link("folder", folder, "page", "", "selected", "") })
	data.AllURL = link("folder", "", "page", "", "selected", "")
	for _, tab := range []string{"documents", "overview", "relations"} {
		data.Tabs = append(data.Tabs, explorerLink{Label: tab, URL: link("tab", tab, "page", "")})
	}
	sort.Slice(docs, func(i, j int) bool {
		a, b := docs[i], docs[j]
		comparison := 0
		switch {
		case data.Sort == "relevance":
			if scores[a.URL] > scores[b.URL] {
				return true
			}
			if scores[a.URL] < scores[b.URL] {
				return false
			}
		case data.Sort == "modified":
			if !a.Modified.Equal(b.Modified) {
				if data.Order == "desc" {
					return a.Modified.After(b.Modified)
				}
				return a.Modified.Before(b.Modified)
			}
		case data.Sort == "title":
			comparison = strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		case strings.HasPrefix(data.Sort, "field:"):
			name := strings.TrimPrefix(data.Sort, "field:")
			av, bv := a.Values[name], b.Values[name]
			if av == "" && bv != "" {
				return false
			}
			if bv == "" && av != "" {
				return true
			}
			if s.siteCfg.Frontmatter.Fields[name].Type == "number" {
				an, _ := strconv.ParseFloat(av, 64)
				bn, _ := strconv.ParseFloat(bv, 64)
				if an < bn {
					comparison = -1
				} else if an > bn {
					comparison = 1
				}
			} else {
				comparison = strings.Compare(av, bv)
			}
		default:
			comparison = strings.Compare(a.Path, b.Path)
		}
		if comparison == 0 {
			return a.Path < b.Path
		}
		if data.Order == "desc" {
			return comparison > 0
		}
		return comparison < 0
	})
	selected := q.Get("selected")
	set := make(map[string]bool)
	for _, doc := range docs {
		set[doc.URL] = true
		if doc.URL == selected {
			data.Selection = doc
		}
	}
	documentURL := func(doc *explorerDocument) string {
		state := cloneExplorerQuery(q)
		values := make(url.Values)
		values.Set("explore", state.Encode())
		return doc.URL + "?" + values.Encode()
	}
	if data.Selection != nil {
		data.SelectionURL = documentURL(data.Selection)
		for _, field := range data.AvailableColumns {
			if value := data.Selection.Values[field.Name]; value != "" {
				data.SelectionValues = append(data.SelectionValues, MetadataFieldData{Label: field.Label, Value: value})
			}
		}
		for _, link := range data.Selection.Incoming {
			data.Incoming = append(data.Incoming, explorerLink{Label: link.Title, URL: documentURL(catalog.Documents[link.FullPath])})
		}
		for _, link := range data.Selection.Outgoing {
			data.Outgoing = append(data.Outgoing, explorerLink{Label: link.Title, URL: documentURL(catalog.Documents[link.FullPath])})
		}
	}
	counts, groups := make(map[string]int), make(map[string]int)
	var edges []explorerEdge
	for _, doc := range docs {
		counts[doc.Folder]++
		if len(doc.Incoming) == 0 {
			data.Orphans++
		}
		for _, target := range doc.Outgoing {
			if !set[target.FullPath] {
				continue
			}
			to := catalog.Documents[target.FullPath]
			edges = append(edges, explorerEdge{From: doc, To: to, FromURL: link("selected", doc.URL), ToURL: link("selected", to.URL)})
			fromFolder, toFolder := strings.Split(doc.Folder, "/")[0], strings.Split(to.Folder, "/")[0]
			if fromFolder == "" {
				fromFolder = "Root"
			}
			if toFolder == "" {
				toFolder = "Root"
			}
			groups[fromFolder+" -> "+toFolder]++
		}
	}
	data.Links = len(edges)
	for folder, count := range counts {
		label := folder
		if label == "" {
			label = "Root"
		}
		data.FolderCounts = append(data.FolderCounts, explorerLink{Label: label, Count: count, URL: link("folder", folder, "page", "", "selected", "")})
	}
	for label, count := range groups {
		data.Groups = append(data.Groups, explorerLink{Label: label, Count: count})
	}
	for _, field := range data.AvailableColumns {
		missing := 0
		for _, doc := range docs {
			if strings.TrimSpace(doc.Values[field.Name]) == "" {
				missing++
			}
		}
		data.Missing = append(data.Missing, explorerLink{Label: field.Label, Count: missing, URL: link("missing", field.Name, "tab", "documents", "page", "", "selected", "")})
	}
	byCount := func(items []explorerLink) {
		sort.Slice(items, func(i, j int) bool {
			if items[i].Count == items[j].Count {
				return items[i].Label < items[j].Label
			}
			return items[i].Count > items[j].Count
		})
	}
	byCount(data.FolderCounts)
	byCount(data.Groups)
	connected := append([]*explorerDocument(nil), docs...)
	sort.Slice(connected, func(i, j int) bool {
		a, b := connected[i], connected[j]
		if len(a.Incoming) == len(b.Incoming) {
			return a.Path < b.Path
		}
		return len(a.Incoming) > len(b.Incoming)
	})
	recent := append([]*explorerDocument(nil), docs...)
	sort.Slice(recent, func(i, j int) bool {
		if recent[i].Modified.Equal(recent[j].Modified) {
			return recent[i].Path < recent[j].Path
		}
		return recent[i].Modified.After(recent[j].Modified)
	})
	for i, doc := range connected {
		if i == 10 {
			break
		}
		data.Connected = append(data.Connected, explorerLink{Label: doc.Title, URL: link("selected", doc.URL), Count: len(doc.Incoming)})
	}
	for i, doc := range recent {
		if i == 10 {
			break
		}
		data.Recent = append(data.Recent, explorerLink{Label: doc.Path, URL: link("selected", doc.URL)})
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	count := len(docs)
	if data.Tab == "relations" {
		count = len(edges)
	}
	data.Pages = (count + 49) / 50
	if data.Pages < 1 {
		data.Pages = 1
	}
	if page > data.Pages {
		page = data.Pages
	}
	data.Page = page
	start, end := (page-1)*50, min(page*50, count)
	if data.Tab == "relations" {
		data.Edges = edges[start:end]
	} else if data.Tab == "documents" {
		for _, doc := range docs[start:end] {
			row := explorerRow{Document: doc, URL: link("selected", doc.URL), Selected: doc.URL == selected}
			for _, column := range data.Columns {
				row.Values = append(row.Values, doc.Values[column.Name])
			}
			data.Rows = append(data.Rows, row)
		}
	}
	if page > 1 {
		data.PreviousURL = link("page", strconv.Itoa(page-1))
	}
	if page < data.Pages {
		data.NextURL = link("page", strconv.Itoa(page+1))
	}
	state := cloneExplorerQuery(q)
	state.Set("page", strconv.Itoa(page))
	for key, values := range state {
		if len(values) == 1 && values[0] == "" {
			state.Del(key)
		}
	}
	data.State = state.Encode()
	w.Header().Set("Cache-Control", "no-store")
	if err := s.render(w, "explorer.html", data); err != nil {
		slog.Error("explorer template error", "err", err)
	}
}

func explorerFolders(docs []*explorerDocument, selected string, link func(string) string) []*explorerFolder {
	root := &explorerFolder{}
	for _, doc := range docs {
		if doc.Folder == "" {
			continue
		}
		parent := root
		prefix := ""
		for _, name := range strings.Split(doc.Folder, "/") {
			if prefix != "" {
				prefix += "/"
			}
			prefix += name
			var child *explorerFolder
			for _, candidate := range parent.Children {
				if candidate.Name == name {
					child = candidate
					break
				}
			}
			if child == nil {
				child = &explorerFolder{Name: name, URL: link(prefix), Open: selected == prefix || strings.HasPrefix(selected, prefix+"/"), Selected: selected == prefix}
				parent.Children = append(parent.Children, child)
			}
			child.Count++
			parent = child
		}
	}
	var order func(*explorerFolder)
	order = func(node *explorerFolder) {
		sort.Slice(node.Children, func(i, j int) bool { return node.Children[i].Name < node.Children[j].Name })
		for _, child := range node.Children {
			order(child)
		}
	}
	order(root)
	return root.Children
}
