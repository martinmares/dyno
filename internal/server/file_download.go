package server

import (
	"encoding/base64"
	"fmt"
	"html"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const downloadsDir = "_downloads"

var fileDownloadPlaceholderRe = regexp.MustCompile(`<div class="file-download-placeholder" data-file-download="([A-Za-z0-9_-]*)"></div>`)

func (s *Server) renderFileDownloads(htmlStr string) string {
	return fileDownloadPlaceholderRe.ReplaceAllStringFunc(htmlStr, func(placeholder string) string {
		match := fileDownloadPlaceholderRe.FindStringSubmatch(placeholder)
		raw, err := base64.RawURLEncoding.DecodeString(match[1])
		if err != nil {
			return fileDownloadError("Invalid download path")
		}
		rel, err := cleanDownloadPath(string(raw))
		if err != nil {
			return fileDownloadError(err.Error())
		}
		filename, info, err := s.resolveDownloadFile(rel)
		if err != nil {
			return fileDownloadError("File not found: " + path.Base(rel))
		}
		contentType := downloadContentType(filename)
		href := s.downloadPath + "/" + escapePathSegments(rel)
		return `<a class="file-download-card" href="` + html.EscapeString(href) + `" download>` +
			`<span class="file-download-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M12 3v12m0 0 4-4m-4 4-4-4M5 21h14" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg></span>` +
			`<span class="file-download-details"><strong>` + html.EscapeString(filepath.Base(filename)) + `</strong>` +
			`<span>` + html.EscapeString(contentType) + ` · ` + formatFileSize(info.Size()) + `</span></span>` +
			`<span class="file-download-action">Download</span></a>`
	})
}

func (s *Server) fileDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if s.browse != nil {
		http.NotFound(w, r)
		return
	}
	rel, err := cleanDownloadPath(r.PathValue("filepath"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	filename, info, err := s.resolveDownloadFile(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", downloadContentType(filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(filename)}))
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, filepath.Base(filename), info.ModTime(), file)
}

func (s *Server) resolveDownloadFile(rel string) (string, os.FileInfo, error) {
	contentRoot, err := filepath.EvalSymlinks(s.contentDir)
	if err != nil {
		return "", nil, err
	}
	downloadRoot, err := filepath.EvalSymlinks(filepath.Join(s.contentDir, downloadsDir))
	if err != nil {
		return "", nil, err
	}
	if !pathWithin(contentRoot, downloadRoot) {
		return "", nil, fmt.Errorf("downloads directory escapes site")
	}
	target, err := filepath.EvalSymlinks(filepath.Join(downloadRoot, filepath.FromSlash(rel)))
	if err != nil {
		return "", nil, err
	}
	if !pathWithin(downloadRoot, target) {
		return "", nil, fmt.Errorf("download escapes downloads directory")
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("download is not a regular file")
	}
	return target, info, nil
}

func cleanDownloadPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.ContainsAny(raw, "\\\x00\r\n") {
		return "", fmt.Errorf("Invalid download path")
	}
	raw = strings.TrimPrefix(raw, "/")
	raw = strings.TrimPrefix(raw, downloadsDir+"/")
	clean := path.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("Invalid download path")
	}
	return clean, nil
}

func isDownloadAssetPath(raw string) bool {
	clean := strings.TrimPrefix(path.Clean("/"+raw), "/")
	return clean == downloadsDir || strings.HasPrefix(clean, downloadsDir+"/")
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func downloadContentType(filename string) string {
	if value := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); value != "" {
		return value
	}
	return "application/octet-stream"
}

func escapePathSegments(value string) string {
	parts := strings.Split(value, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func formatFileSize(size int64) string {
	const unit = int64(1024)
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := unit, 0
	for n := size / unit; n >= unit && exp < 3; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGT"[exp])
}

func fileDownloadError(message string) string {
	return `<div class="file-download-error">` + html.EscapeString(message) + `</div>`
}
