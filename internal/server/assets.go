package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

type assetMetadata struct {
	etag string
	hash string
}

func buildAssetManifest(assetsFS fs.FS) (map[string]assetMetadata, error) {
	manifest := make(map[string]assetMetadata)
	err := fs.WalkDir(assetsFS, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(assetsFS, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		manifest[name] = assetMetadata{
			etag: `"` + hash + `"`,
			hash: hash[:12],
		}
		return nil
	})
	return manifest, err
}

func (s *Server) assetURL(name string) string {
	clean := strings.TrimPrefix(path.Clean("/assets/"+strings.TrimPrefix(name, "/assets/")), "/")
	urlPath := "/" + clean
	if s.devMode {
		return urlPath
	}
	meta, ok := s.assetMeta[strings.TrimPrefix(urlPath, "/assets/")]
	if !ok {
		return urlPath
	}
	return urlPath + "?v=" + meta.hash
}

func (s *Server) faviconURL() string {
	if s.siteCfg == nil || s.siteCfg.Favicon == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(s.siteCfg.Favicon, "/assets/"):
		return s.assetURL(strings.TrimPrefix(s.siteCfg.Favicon, "/assets/"))
	case strings.HasPrefix(s.siteCfg.Favicon, "assets/"):
		return s.assetURL(strings.TrimPrefix(s.siteCfg.Favicon, "assets/"))
	default:
		return s.siteCfg.Favicon
	}
}

func (s *Server) assetHandler(assetsFS fs.FS) http.Handler {
	fileServer := http.StripPrefix("/assets/", http.FileServer(http.FS(assetsFS)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.devMode {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			fileServer.ServeHTTP(w, r)
			return
		}

		name := strings.TrimPrefix(path.Clean(strings.TrimPrefix(r.URL.Path, "/assets/")), "/")
		if meta, ok := s.assetMeta[name]; ok {
			w.Header().Set("ETag", meta.etag)
			if !s.buildTime.IsZero() {
				w.Header().Set("Last-Modified", s.buildTime.UTC().Format(http.TimeFormat))
			}
			if isConditionalRequestFresh(r, meta.etag, s.buildTime) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		fileServer.ServeHTTP(w, r)
	})
}

func isConditionalRequestFresh(r *http.Request, etag string, modTime time.Time) bool {
	if match := r.Header.Get("If-None-Match"); match != "" && etagMatches(match, etag) {
		return true
	}
	if modTime.IsZero() {
		return false
	}
	if since := r.Header.Get("If-Modified-Since"); since != "" {
		t, err := time.Parse(http.TimeFormat, since)
		if err == nil && !modTime.After(t.Add(time.Second)) {
			return true
		}
	}
	return false
}

func etagMatches(headerValue, etag string) bool {
	for _, token := range strings.Split(headerValue, ",") {
		candidate := strings.TrimSpace(token)
		if candidate == etag || candidate == "W/"+etag || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}
