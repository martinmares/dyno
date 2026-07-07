package markdown

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

var fileDownloadFenceRe = regexp.MustCompile("(?ms)^```file-download\\s*\n(.*?)\n```")

func replaceFileDownloadsWithPlaceholders(src []byte, blocks map[string]string) []byte {
	counter := 0
	return fileDownloadFenceRe.ReplaceAllFunc(src, func(match []byte) []byte {
		subs := fileDownloadFenceRe.FindSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		key := fmt.Sprintf("FILEDOWNLOADPLACEHOLDER%d", counter)
		counter++
		blocks[key] = strings.TrimSpace(string(subs[1]))
		return []byte("<div>" + key + "</div>")
	})
}

func restoreFileDownloadPlaceholders(htmlStr string, blocks map[string]string) string {
	for key, filePath := range blocks {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(filePath))
		placeholder := `<div class="file-download-placeholder" data-file-download="` + encoded + `"></div>`
		htmlStr = strings.ReplaceAll(htmlStr, "<div>"+key+"</div>", placeholder)
	}
	return htmlStr
}
