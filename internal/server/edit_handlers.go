package server

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
)

// EditPageData is passed to the edit.html template.
type EditPageData struct {
	PageData
	FSPath      string        // absolute filesystem path of the file being edited
	PageURL     string        // dyno URL of the page (for "View page" link)
	Source      string        // raw markdown source
	PreviewHTML template.HTML // initial rendered preview
	PreviewURL  string        // POST endpoint for live preview
	SaveURL     string        // POST endpoint for saving
}

// editPath returns the /_edit base path for this server.
func (s *Server) editBasePath() string {
	if s.basePath != "" {
		return s.basePath + "/_edit"
	}
	return "/_edit"
}

// editPageURL returns the editor URL for a given page URL path.
func (s *Server) editPageURLFor(pageFullPath string) string {
	suffix := strings.TrimPrefix(pageFullPath, s.basePath)
	return s.editBasePath() + suffix
}

// editPageHandler serves the split-screen editor for a page.
// Route: GET /_edit/{pagepath...}
func (s *Server) editPageHandler(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, s.editBasePath())
	pageURL := s.basePath + suffix
	pageURL = strings.TrimRight(pageURL, "/")

	node := navigation.FindNode(s.getNav(), pageURL)
	if node == nil {
		node = navigation.FindNode(s.getNav(), pageURL+"/")
	}
	if node == nil || node.FSPath == "" || node.IsDir {
		s.notFound(w, r)
		return
	}

	src, err := os.ReadFile(node.FSPath)
	if err != nil {
		s.internalError(w, r, fmt.Errorf("read file: %w", err))
		return
	}

	// Render initial preview.
	res, err := s.renderer.RenderWithOptions([]byte(src), markdown.RenderOptions{TaskRenderer: s.renderTasks})
	if err != nil {
		s.internalError(w, r, fmt.Errorf("render: %w", err))
		return
	}

	previewURL := s.editBasePath() + "/preview"
	saveURL := s.editBasePath() + "/save"

	data := EditPageData{
		PageData: PageData{
			Title:        "Editace — " + node.Title,
			Nav:          s.getNav(),
			CurrentPath:  node.FullPath,
			Breadcrumbs:  navigation.Breadcrumbs(s.getNav(), node.FullPath),
			LightCSS:     template.CSS(s.renderer.LightCSS()),
			DarkCSS:      template.CSS(s.renderer.DarkCSS()),
			Site:         s.siteCfg,
			BasePath:     s.basePath,
			TailwindURL:  s.assetURL("tailwind.css"),
			AppCSSURL:    s.assetURL("app.css"),
			AppJSURL:     s.assetURL("app.js"),
			HTMXURL:      s.assetURL("htmx.min.js"),
			MermaidURL:   s.assetURL("mermaid.min.js"),
			FaviconURL:   s.faviconURL(),
			SearchURL:    s.searchPath,
			TasksURL:     s.tasksPath,
			LibraryURL:   s.libraryURL,
			BuildVersion: s.version,
			BuildCommit:  s.commit,
			EditMode:     true,
			EditPageURL:  s.editPageURLFor(node.FullPath),
		},
		FSPath:      node.FSPath,
		PageURL:     node.FullPath,
		Source:      string(src),
		PreviewHTML: template.HTML(res.HTML),
		PreviewURL:  previewURL,
		SaveURL:     saveURL,
	}

	if err := s.render(w, "edit.html", data); err != nil {
		slog.Error("template error", "template", "edit.html", "err", err)
	}
}

// editPreviewHandler renders markdown source to HTML (for live preview).
// Route: POST /_edit/preview
func (s *Server) editPreviewHandler(w http.ResponseWriter, r *http.Request) {
	src := r.FormValue("source")
	res, err := s.renderer.RenderWithOptions([]byte(src), markdown.RenderOptions{TaskRenderer: s.renderTasks})
	if err != nil {
		http.Error(w, "render error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, res.HTML)
}

// editSaveHandler writes the edited markdown source to disk.
// Route: POST /_edit/save
func (s *Server) editSaveHandler(w http.ResponseWriter, r *http.Request) {
	fsPath := r.FormValue("fs_path")
	src := r.FormValue("source")

	// Security: ensure fsPath is within contentDir.
	if !strings.HasPrefix(fsPath, s.contentDir) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := os.WriteFile(fsPath, []byte(src), 0o644); err != nil {
		http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Invalidate page cache for this file.
	s.mu.Lock()
	delete(s.pageCache, fsPath)
	s.mu.Unlock()

	slog.Info("page saved", "path", fsPath)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}
