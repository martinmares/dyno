package server

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/mares/dyno/internal/comments"
	"github.com/mares/dyno/internal/navigation"
	"golang.org/x/text/unicode/norm"
)

type overviewComment struct {
	Comment    comments.Comment
	BodyHTML   template.HTML
	URL        string
	ContextURL string
}

type commentDocumentGroup struct {
	Title, Path, URL string
	UpdatedAt        time.Time
	Entries          []overviewComment
}

type commentsOverviewData struct {
	URL, Query   string
	Total, Shown int
	Groups       []*commentDocumentGroup
}

func (s *Server) commentsOverviewHandler(w http.ResponseWriter, r *http.Request) {
	if s.comments == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	items, err := s.comments.ListAll()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	view := s.buildCommentsOverview(items, r.URL.Query().Get("q"))
	tmpl, err := s.getTemplate()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if isHTMX(r) && r.Header.Get("HX-Target") == "comments-overview-results" {
		if err := tmpl.ExecuteTemplate(w, "comments-overview-results", view); err != nil {
			slog.Error("comments overview template error", "err", err)
		}
		return
	}
	var content bytes.Buffer
	if err := tmpl.ExecuteTemplate(&content, "comments-overview-content", view); err != nil {
		s.internalError(w, r, err)
		return
	}
	data := PageData{
		Title: "Local comments", ContentHTML: template.HTML(content.String()),
		Nav: s.getNav(), Site: s.siteCfg, BasePath: s.basePath,
		CurrentPath: s.commentsPath, LibraryURL: s.libraryURL,
		LightCSS: template.CSS(s.renderer.LightCSS()), DarkCSS: template.CSS(s.renderer.DarkCSS()),
		AppCSSURL: s.assetURL("app.css"), AppJSURL: s.assetURL("app.js"),
		HTMXURL: s.assetURL("htmx.min.js"), MermaidURL: s.assetURL("mermaid.min.js"),
		FaviconURL: s.faviconURL(), SearchURL: s.searchPath,
		TasksURL: s.tasksPath, GraphURL: s.graphPath, GitHistoryURL: s.gitHistoryPath,
		BuildVersion: s.version, BuildCommit: s.commit,
	}
	name := "base.html"
	if isHTMX(r) {
		name = "page-fragment"
	}
	if err := s.render(w, name, data); err != nil {
		slog.Error("comments overview template error", "err", err)
	}
}

func (s *Server) buildCommentsOverview(items []comments.Comment, query string) commentsOverviewData {
	data := commentsOverviewData{URL: s.commentsPath, Query: strings.TrimSpace(query)}
	byID := make(map[string]*navigation.NavNode)
	byPath := make(map[string]*navigation.NavNode)
	wanted := make(map[string]bool)
	for _, item := range items {
		if item.Kind != "highlight" {
			wanted[item.DocumentID] = true
		}
	}
	if len(wanted) > 0 {
		navigation.WalkNodes(s.getNav(), func(node *navigation.NavNode) {
			if node.FSPath == "" {
				return
			}
			byPath[node.FullPath] = node
			id := s.commentDocumentIDFromFile(node.FSPath)
			if id != "" && wanted[id] {
				byID[id] = node
			}
		})
	}
	groups := make(map[string]*commentDocumentGroup)
	terms := strings.Fields(foldCommentSearch(data.Query))
	for _, item := range items {
		if item.Kind == "highlight" {
			continue
		}
		data.Total++
		key := "id:" + item.DocumentID
		node := byID[item.DocumentID]
		if item.DocumentID == "" {
			key = "path:" + item.PagePath
			node = byPath[item.PagePath]
		}
		title, pagePath, pageURL := item.PagePath, item.PagePath, ""
		if node != nil {
			title, pagePath, pageURL = node.Title, node.FullPath, node.FullPath
			key = "path:" + node.FullPath
		}
		if title == "" {
			title = "Unavailable document"
		}
		haystack := foldCommentSearch(strings.Join([]string{title, pagePath, item.Author, item.Body, item.Quote}, " "))
		matches := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		data.Shown++
		group := groups[key]
		if group == nil {
			group = &commentDocumentGroup{Title: title, Path: pagePath, URL: pageURL}
			groups[key] = group
			data.Groups = append(data.Groups, group)
		}
		updated := item.CreatedAt
		if item.UpdatedAt.After(updated) {
			updated = item.UpdatedAt
		}
		if updated.After(group.UpdatedAt) {
			group.UpdatedAt = updated
		}
		entry := overviewComment{Comment: item, BodyHTML: renderCommentMarkdown(item.Body)}
		if pageURL != "" {
			entry.URL = commentOverviewLink(pageURL, "comment-"+item.ID)
			if item.Anchor != "" {
				entry.ContextURL = commentOverviewLink(pageURL, item.Anchor)
			}
		}
		group.Entries = append(group.Entries, entry)
	}
	sort.Slice(data.Groups, func(i, j int) bool {
		if data.Groups[i].UpdatedAt.Equal(data.Groups[j].UpdatedAt) {
			return data.Groups[i].Path < data.Groups[j].Path
		}
		return data.Groups[i].UpdatedAt.After(data.Groups[j].UpdatedAt)
	})
	return data
}

func commentOverviewLink(pagePath, fragment string) string {
	u, err := url.Parse(pagePath)
	if err != nil {
		return ""
	}
	u.Fragment = fragment
	return u.String()
}

func foldCommentSearch(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return unicode.ToLower(r)
	}, norm.NFD.String(value))
}
