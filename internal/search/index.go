package search

import (
	"html"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/blevesearch/bleve/v2"
	"github.com/mares/dyno/internal/navigation"
	"golang.org/x/text/unicode/norm"
)

// Document is a single indexed page.
type Document struct {
	Path     string // URL path
	Title    string
	Body     string   // Plain text, for snippet extraction
	Headings []string // H2/H3 headings extracted from markdown source
	Sections []Section
}

// Options controls the full result set used by Explorer. Quick search remains
// limited to twenty results through SearchFiltered.
type Options struct {
	Match        string // all (default) or any
	Scope        string // all (default), title, path or body
	Limit        int    // zero returns the full result set
	Offset       int    // document offset, applied after relevance sorting
	OmitSnippets bool   // Explorer has a separate selected-document preview
}

// Index is an in-memory full-text search index.
type Index struct {
	mu     sync.RWMutex
	docs   []Document
	engine bleve.Index
	err    error
}

// SearchResult is one search hit.
type SearchResult struct {
	Path        string
	Title       string
	Snippet     string // Plain text with <mark> around matches
	Score       float64
	BookTitle   string // Set by library mode to identify which book the result is from
	BookSlug    string // Set by library mode for badge coloring
	Section     string // Breadcrumb of the most relevant chapter
	Anchor      string // Heading fragment, separate from Path for metadata and Explorer
	Approximate bool   // The result needed typo tolerance
	Partial     bool   // Only some query words matched
}

func fold(text string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return unicode.ToLower(r)
	}, norm.NFD.String(text))), " ")
}

// New creates an empty index.
func New() *Index {
	engine, err := newEngine()
	return &Index{engine: engine, err: err}
}

// Add indexes a document.
func (idx *Index) Add(doc Document) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.err != nil {
		return
	}
	idx.err = idx.engine.Index(doc.Path, engineDocument(doc))
	if idx.err != nil {
		return
	}
	for i := range idx.docs {
		if idx.docs[i].Path == doc.Path {
			idx.docs[i] = doc
			return
		}
	}
	idx.docs = append(idx.docs, doc)
}

// extractHeadings returns H2/H3 headings from raw markdown source (max 4).
func extractHeadings(src string) []string {
	return previewHeadings(extractSections([]byte(src)))
}

func previewHeadings(sections []Section) []string {
	var out []string
	for _, section := range sections {
		if section.Level == 2 || section.Level == 3 {
			out = append(out, section.Heading)
			if len(out) == 4 {
				break
			}
		}
	}
	return out
}

var headingFrontmatter = regexp.MustCompile(`(?s)^(?:\x{FEFF})?---\r?\n.*?\r?\n---(?:\r?\n)?`)

// BuildIndex walks all .md files in the nav tree, renders them to plain text,
// and adds them to a new Index.
func BuildIndex(tree *navigation.NavNode, plainText func(path string) (string, error)) (*Index, error) {
	return BuildIndexWithReader(tree, plainText, os.ReadFile)
}

// BuildIndexWithReader lets a local browser confine indexing to selected roots.
func BuildIndexWithReader(tree *navigation.NavNode, plainText func(string) (string, error), readFile func(string) ([]byte, error)) (*Index, error) {
	idx := New()
	walkNodes(tree, func(node *navigation.NavNode) {
		if node.FSPath == "" {
			return
		}
		src, err := readFile(node.FSPath)
		if err != nil {
			return
		}
		body, err := plainText(string(src))
		if err != nil {
			return
		}
		sections := extractSections(src)
		idx.Add(Document{
			Path:     node.FullPath,
			Title:    node.Title,
			Body:     body,
			Headings: previewHeadings(sections),
			Sections: sections,
		})
	})
	return idx, idx.err
}

func walkNodes(node *navigation.NavNode, fn func(*navigation.NavNode)) {
	fn(node)
	for _, child := range node.Children {
		walkNodes(child, fn)
	}
}

// DocCount returns the number of indexed documents.
func (idx *Index) DocCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.docs)
}

// Headings returns H2/H3 headings for the document at the given URL path.
func (idx *Index) Headings(urlPath string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	for _, doc := range idx.docs {
		if doc.Path == urlPath {
			return doc.Headings
		}
	}
	return nil
}

// Search returns the top results for a query string.
func (idx *Index) Search(query string) []SearchResult {
	return idx.SearchFiltered(query, nil)
}

// SearchFiltered restricts results to paths accepted by allowed. A nil map
// searches every indexed document.
func (idx *Index) SearchFiltered(query string, allowed map[string]bool) []SearchResult {
	results, _ := idx.SearchPage(query, allowed, 0, 20)
	return results
}

// SearchPage returns a relevance-ranked document page and the total hit count.
func (idx *Index) SearchPage(query string, allowed map[string]bool, offset, limit int) ([]SearchResult, int) {
	if strings.TrimSpace(query) == "" {
		return nil, 0
	}
	results, total := idx.query(query, Options{Limit: limit, Offset: offset}, allowed)
	if total == 0 {
		results, total = idx.query(query, Options{Match: "any", Limit: limit, Offset: offset}, allowed)
		for i := range results {
			results[i].Partial = true
		}
	}
	return results, total
}

// Documents returns a snapshot; callers must not mutate the heading slices.
func (idx *Index) Documents() []Document {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return append([]Document(nil), idx.docs...)
}

var queryClause = regexp.MustCompile(`"([^"]+)"|(\S+)`)

// Query searches phrases and words without imposing the quick-search limit.
func (idx *Index) Query(query string, options Options, allowed map[string]bool) []SearchResult {
	results, _ := idx.query(query, options, allowed)
	return results
}

func (idx *Index) query(query string, options Options, allowed map[string]bool) ([]SearchResult, int) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.err != nil {
		slog.Error("search index unavailable", "err", idx.err)
		return nil, 0
	}
	return idx.search(query, options, allowed)
}

// extractSnippet prefers a dense matching passage over an incidental first mention.
func extractSnippet(body string, tokens []string, _ int) string {
	runes := []rune(body)
	runePos := 0
	marks := matchRunes(body, tokens)
	prefix := make([]int, len(marks)+1)
	for i, marked := range marks {
		prefix[i+1] = prefix[i]
		if marked {
			prefix[i+1]++
		}
	}
	bestScore := 0
	for i, marked := range marks {
		if !marked {
			continue
		}
		start, end := max(0, i-80), min(len(runes), i+150)
		if score := prefix[end] - prefix[start]; score > bestScore {
			bestScore, runePos = score, i
		}
	}

	start := runePos - 80
	if start < 0 {
		start = 0
	}
	end := runePos + 150
	if end > len(runes) {
		end = len(runes)
	}
	minStart, maxEnd := max(0, start-32), min(len(runes), end+32)
	for start > minStart && start < runePos && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	for end < maxEnd && !unicode.IsSpace(runes[end]) {
		end++
	}

	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet = snippet + "…"
	}

	return highlightTerms(snippet, tokens)
}

// highlightTerms wraps each occurrence of any token (or word containing it) in <mark>...</mark>.
func highlightTerms(text string, tokens []string) string {
	marked := matchRunes(text, tokens)
	var out strings.Builder
	active := false
	for i, r := range []rune(text) {
		if marked[i] != active {
			if active {
				out.WriteString("</mark>")
			} else {
				out.WriteString("<mark>")
			}
			active = marked[i]
		}
		out.WriteString(html.EscapeString(string(r)))
	}
	if active {
		out.WriteString("</mark>")
	}
	return out.String()
}

// Map normalized characters back to source runes so accents and HTML escaping
// cannot shift highlights or introduce markup from a document.
func matchRunes(text string, terms []string) []bool {
	runes := []rune(text)
	marked := make([]bool, len(runes))
	if len(terms) == 0 {
		return marked
	}
	var normalized strings.Builder
	var positions []int
	space := false
	for i, r := range runes {
		part := string(r)
		if r >= 128 {
			part = norm.NFD.String(part)
		}
		for _, n := range part {
			if unicode.Is(unicode.Mn, n) {
				continue
			}
			if unicode.IsSpace(n) {
				if space {
					continue
				}
				n = ' '
				space = true
			} else {
				space = false
			}
			part := string(unicode.ToLower(n))
			normalized.WriteString(part)
			for range len(part) {
				positions = append(positions, i)
			}
		}
	}
	body := normalized.String()
	for _, term := range terms {
		if term == "" {
			continue
		}
		for offset := 0; offset < len(body); {
			index := strings.Index(body[offset:], term)
			if index < 0 {
				break
			}
			index += offset
			start, end := positions[index], positions[index+len(term)-1]+1
			if searchWordRune(runes[start]) {
				for start > 0 && searchWordRune(runes[start-1]) {
					start--
				}
			}
			if searchWordRune(runes[end-1]) {
				for end < len(runes) && searchWordRune(runes[end]) {
					end++
				}
			}
			for i := start; i < end; i++ {
				marked[i] = true
			}
			offset = index + len(term)
		}
	}
	for i := 1; i < len(runes); i++ {
		if unicode.Is(unicode.Mn, runes[i]) && marked[i-1] {
			marked[i] = true
		}
	}
	return marked
}

func searchWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_'
}
