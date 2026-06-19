package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/mares/dyno/internal/comments"
	"github.com/mares/dyno/internal/navigation"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var commentMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(html.WithHardWraps()),
)

func (s *Server) listComments(pagePath string) ([]comments.Comment, error) {
	if s.comments == nil {
		return nil, nil
	}
	return s.comments.List(pagePath)
}

func (s *Server) addCommentHandler(w http.ResponseWriter, r *http.Request) {
	if s.comments == nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	pagePath := strings.TrimSpace(r.FormValue("page_path"))
	if pagePath == "" {
		http.Error(w, "page_path is required", http.StatusBadRequest)
		return
	}
	if !s.isKnownPagePath(pagePath) {
		http.Error(w, "unknown page_path", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		http.Error(w, "body is required", http.StatusBadRequest)
		return
	}
	parentID := strings.TrimSpace(r.FormValue("parent_id"))
	if parentID != "" && !s.commentExists(pagePath, parentID) {
		http.Error(w, "unknown parent_id", http.StatusBadRequest)
		return
	}

	comment, err := s.comments.Add(comments.Comment{
		PagePath: pagePath,
		ParentID: parentID,
		Anchor:   strings.TrimSpace(r.FormValue("anchor")),
		Quote:    strings.TrimSpace(r.FormValue("quote")),
		Author:   s.commentAuthor(r),
		Body:     body,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, comments.ErrValidation) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}

	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(comment)
		return
	}
	http.Redirect(w, r, pagePath+"#comments", http.StatusSeeOther)
}

func (s *Server) isKnownPagePath(pagePath string) bool {
	return navigation.FindNode(s.getNav(), pagePath) != nil
}

func (s *Server) commentExists(pagePath, id string) bool {
	existing, err := s.listComments(pagePath)
	if err != nil {
		return false
	}
	for _, comment := range existing {
		if comment.ID == id {
			return true
		}
	}
	return false
}

func (s *Server) commentAuthor(r *http.Request) string {
	for _, name := range []string{
		"X-Auth-Request-User",
		"X-Forwarded-User",
		"Remote-User",
	} {
		if value := strings.TrimSpace(r.Header.Get(name)); value != "" {
			return value
		}
	}
	return strings.TrimSpace(r.FormValue("author"))
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.Contains(r.Header.Get("Content-Type"), "application/json")
}

func renderCommentMarkdown(src string) template.HTML {
	var buf bytes.Buffer
	if err := commentMarkdown.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(buf.String())
}

func buildCommentTree(items []comments.Comment, render func(string) template.HTML) []CommentNode {
	children := make(map[string][]comments.Comment)
	known := make(map[string]bool, len(items))
	for _, item := range items {
		known[item.ID] = true
	}
	for _, item := range items {
		parentID := item.ParentID
		if parentID != "" && !known[parentID] {
			parentID = ""
		}
		children[parentID] = append(children[parentID], item)
	}
	for parentID := range children {
		sort.SliceStable(children[parentID], func(i, j int) bool {
			return children[parentID][i].CreatedAt.Before(children[parentID][j].CreatedAt)
		})
	}

	var walk func(parentID string, depth int) []CommentNode
	walk = func(parentID string, depth int) []CommentNode {
		items := children[parentID]
		nodes := make([]CommentNode, 0, len(items))
		for _, item := range items {
			nodes = append(nodes, CommentNode{
				Comment:  item,
				BodyHTML: render(item.Body),
				Children: walk(item.ID, depth+1),
				Depth:    depth,
			})
		}
		return nodes
	}
	return walk("", 0)
}
