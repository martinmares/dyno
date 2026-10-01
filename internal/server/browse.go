package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mares/dyno/internal/navigation"
	xhtml "golang.org/x/net/html"
)

type browseAsset struct{ document, path string }

func (s *Server) readDocument(name string) ([]byte, error) {
	if s.browse != nil {
		return s.browse.ReadFile(name)
	}
	return os.ReadFile(name)
}

func (s *Server) documentStat(name string) (os.FileInfo, error) {
	if s.browse != nil {
		return s.browse.Stat(name)
	}
	return os.Stat(name)
}

func (s *Server) writeDocument(name string, data []byte, mode os.FileMode) error {
	if s.browse != nil {
		return s.browse.WriteFile(name, data, mode)
	}
	return writeFileAtomic(name, data, mode)
}

func (s *Server) rewriteBrowseLinks(fragment, document string) string {
	z := xhtml.NewTokenizer(strings.NewReader(fragment))
	var out strings.Builder
	for {
		kind := z.Next()
		if kind == xhtml.ErrorToken {
			break
		}
		raw := string(z.Raw())
		if kind != xhtml.StartTagToken && kind != xhtml.SelfClosingTagToken {
			out.WriteString(raw)
			continue
		}
		token := z.Token()
		changed := false
		blocked := false
		for i, attr := range token.Attr {
			if attr.Key != "href" && attr.Key != "src" && attr.Key != "poster" {
				continue
			}
			u, err := url.Parse(attr.Val)
			if err != nil {
				continue
			}
			if u.Scheme != "" || u.Host != "" {
				if u.Scheme != "file" {
					continue
				}
			} else if u.Path == "" {
				continue
			}
			target, err := s.browse.Resolve(document, u.Path)
			value := s.basePath + "/_blocked"
			if err == nil && u.Scheme == "" {
				nav := s.getNav()
				node := navigation.FindNodeByFSPath(nav, target)
				if node == nil {
					if path := s.browse.NavigationPath(target); path != "" {
						node = navigation.FindNode(nav, path)
					}
				}
				if node != nil {
					if node.IsDir {
						value = node.FullPath
					} else if _, err := s.documentStat(target); err == nil {
						value = node.FullPath
					}
				} else if asset, err := s.browse.OpenAsset(document, target); err == nil {
					asset.Close()
					id := fmt.Sprintf("%x", sha256.Sum256([]byte(document+"\x00"+target)))
					s.mu.Lock()
					s.browseAssets[id] = browseAsset{document: document, path: target}
					s.mu.Unlock()
					value = s.basePath + "/_asset/" + id
				}
			}
			if value == s.basePath+"/_blocked" {
				blocked = true
			}
			if token.Data == "img" && attr.Key == "src" {
				token.Attr = append(token.Attr, xhtml.Attribute{Key: "data-source-src", Val: attr.Val})
			}
			u.Path, _ = url.PathUnescape(value)
			u.RawPath = value
			u.Scheme, u.Host, u.Opaque = "", "", ""
			token.Attr[i].Val = u.String()
			changed = true
		}
		if blocked {
			token.Attr = append(token.Attr, xhtml.Attribute{Key: "title", Val: "Local target is not available in the selected sources. Add its file or directory to dyno browse."})
		}
		if changed {
			out.WriteString(token.String())
		} else {
			out.WriteString(raw)
		}
	}
	return out.String()
}

func (s *Server) browseAssetHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	asset, ok := s.browseAssets[r.PathValue("id")]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := s.browse.OpenAsset(asset.document, asset.path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := downloadContentType(asset.path)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("Cache-Control", "no-cache")
	if !strings.HasPrefix(contentType, "image/") {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(asset.path)}))
	}
	http.ServeContent(w, r, filepath.Base(asset.path), info.ModTime(), f)
}

func (s *Server) browseStatusHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	revision := s.browseRevision
	s.mu.RUnlock()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]uint64{"revision": revision})
}

func (s *Server) browseBlockedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	data := PageData{
		Title: "Local target is not selected", Nav: s.getNav(), Site: s.siteCfg, BasePath: s.basePath,
		ContentHTML: template.HTML(`<p>This local link is missing, excluded, or outside the selected sources. Add its Markdown file or containing directory to <code>dyno browse</code> to make it available.</p>`),
		LightCSS:    template.CSS(s.renderer.LightCSS()), DarkCSS: template.CSS(s.renderer.DarkCSS()),
		AppCSSURL: s.assetURL("app.css"), AppJSURL: s.assetURL("app.js"), HTMXURL: s.assetURL("htmx.min.js"), MermaidURL: s.assetURL("mermaid.min.js"), SearchURL: s.searchPath,
	}
	if isHTMX(r) {
		_ = s.render(w, "page-fragment", data)
	} else {
		_ = s.render(w, "base.html", data)
	}
}
