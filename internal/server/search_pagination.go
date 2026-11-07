package server

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const searchPageSize = 20

func searchOffset(r *http.Request) int {
	if isHTMX(r) {
		return 0
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	return min(max(page, 0), 10000) * searchPageSize
}

func setSearchPagination(data *SearchData, r *http.Request, total, offset int) {
	data.TotalResults = total
	current, _ := url.Parse(r.Header.Get("HX-Current-URL"))
	data.CanSwapResults = isHTMX(r) && (current == nil || !strings.HasSuffix(current.Path, "/_search"))
	data.RangeEnd = offset + len(data.Results)
	if len(data.Results) > 0 {
		data.RangeStart = offset + 1
	}
	values := r.URL.Query()
	values.Del("page")
	data.AllResultsURL = r.URL.Path + "?" + values.Encode()
	if offset > 0 {
		values.Set("page", strconv.Itoa(offset/searchPageSize-1))
		data.PreviousURL = r.URL.Path + "?" + values.Encode()
	}
	if offset+len(data.Results) < total {
		values.Set("page", strconv.Itoa(offset/searchPageSize+1))
		data.NextURL = r.URL.Path + "?" + values.Encode()
	}
}
