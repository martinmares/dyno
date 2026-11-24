package server

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	xhtml "golang.org/x/net/html"
)

// rewriteSiteAssetLinks resolves assets from the source file, not its slugged page URL.
func (s *Server) rewriteSiteAssetLinks(fragment, sourcePath string) string {
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
		for i, attr := range token.Attr {
			if attr.Key != "href" && attr.Key != "src" && attr.Key != "poster" {
				continue
			}
			u, err := url.Parse(attr.Val)
			if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") || isMarkdownPath(u.Path) {
				continue
			}
			target := filepath.Join(filepath.Dir(sourcePath), filepath.FromSlash(u.Path))
			if !pathWithin(s.contentDir, target) {
				continue
			}
			rel, err := filepath.Rel(s.contentDir, target)
			if err != nil || isDownloadAssetPath(filepath.ToSlash(rel)) {
				continue
			}
			info, err := os.Stat(target)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			u.Path = s.basePath + "/" + filepath.ToSlash(rel)
			u.RawPath = ""
			token.Attr[i].Val = u.String()
			changed = true
		}
		if changed {
			out.WriteString(token.String())
		} else {
			out.WriteString(raw)
		}
	}
	return out.String()
}
