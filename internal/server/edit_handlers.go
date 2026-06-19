package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
)

// EditPageData is passed to the edit.html template.
type EditPageData struct {
	PageData
	PageURL     string        // dyno URL of the page (for "View page" link)
	Source      string        // raw markdown source
	PreviewHTML template.HTML // initial rendered preview
	PreviewURL  string        // POST endpoint for live preview
	SaveURL     string        // POST endpoint for saving
	Revision    string        // content hash used for optimistic concurrency control
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
	if pageURL == s.basePath {
		pageURL = s.basePath + "/"
	}

	node := navigation.FindNode(s.getNav(), pageURL)
	if node == nil {
		node = navigation.FindNode(s.getNav(), pageURL+"/")
	}
	if node == nil || node.FSPath == "" {
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
			Title:        "Edit - " + node.Title,
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
		PageURL:     node.FullPath,
		Source:      string(src),
		PreviewHTML: template.HTML(res.HTML),
		PreviewURL:  previewURL,
		SaveURL:     saveURL,
		Revision:    contentRevision(src),
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
	pagePath := strings.TrimSpace(r.FormValue("page_path"))
	expectedRevision := strings.TrimSpace(r.FormValue("revision"))
	src := r.FormValue("source")

	if pagePath == "" || expectedRevision == "" {
		writeEditSaveResponse(w, http.StatusBadRequest, editSaveResponse{
			Error: "page_path and revision are required",
			Code:  "invalid_request",
		})
		return
	}

	fsPath, err := s.editableFilePath(pagePath)
	if err != nil {
		writeEditSaveResponse(w, http.StatusNotFound, editSaveResponse{
			Error: "document not found",
			Code:  "not_found",
		})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := os.ReadFile(fsPath)
	if err != nil {
		writeEditSaveResponse(w, http.StatusInternalServerError, editSaveResponse{
			Error: "failed to read document",
			Code:  "read_failed",
		})
		return
	}
	currentRevision := contentRevision(current)
	newContent := []byte(src)
	newRevision := contentRevision(newContent)
	if currentRevision == newRevision {
		// Treat unchanged content and retried saves as successful without rewriting the file.
		writeEditSaveResponse(w, http.StatusOK, editSaveResponse{OK: true, Revision: currentRevision})
		return
	}
	if currentRevision != expectedRevision {
		writeEditSaveResponse(w, http.StatusConflict, editSaveResponse{
			Error:    "document changed on disk; reload before saving",
			Code:     "conflict",
			Revision: currentRevision,
		})
		return
	}

	info, err := os.Stat(fsPath)
	if err != nil {
		writeEditSaveResponse(w, http.StatusInternalServerError, editSaveResponse{
			Error: "failed to inspect document",
			Code:  "stat_failed",
		})
		return
	}
	if err := writeFileAtomic(fsPath, newContent, info.Mode().Perm()); err != nil {
		writeEditSaveResponse(w, http.StatusInternalServerError, editSaveResponse{
			Error: "failed to write document",
			Code:  "write_failed",
		})
		return
	}

	delete(s.pageCache, fsPath)

	slog.Info("page saved", "path", fsPath)
	writeEditSaveResponse(w, http.StatusOK, editSaveResponse{OK: true, Revision: newRevision})
}

type editSaveResponse struct {
	OK       bool   `json:"ok"`
	Revision string `json:"revision,omitempty"`
	Error    string `json:"error,omitempty"`
	Code     string `json:"code,omitempty"`
}

func writeEditSaveResponse(w http.ResponseWriter, status int, response editSaveResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func contentRevision(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func (s *Server) editableFilePath(pagePath string) (string, error) {
	node := navigation.FindNode(s.getNav(), pagePath)
	if node == nil {
		node = navigation.FindNode(s.getNav(), strings.TrimRight(pagePath, "/")+"/")
	}
	if node == nil || node.FSPath == "" {
		return "", os.ErrNotExist
	}

	contentDir, err := filepath.Abs(s.contentDir)
	if err != nil {
		return "", err
	}
	fsPath, err := filepath.Abs(node.FSPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(contentDir, fsPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", os.ErrPermission
	}
	if !strings.EqualFold(filepath.Ext(fsPath), ".md") {
		return "", os.ErrPermission
	}
	return fsPath, nil
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dyno-edit-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
