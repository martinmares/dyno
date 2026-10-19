package search

import (
	"html"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/mares/dyno/internal/navigation"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/text/unicode/norm"
)

// Document is a single indexed page.
type Document struct {
	Path     string // URL path
	Title    string
	Body     string   // Plain text, for snippet extraction
	Headings []string // H2/H3 headings extracted from markdown source
}

// Options controls the full result set used by Explorer. Quick search remains
// limited to twenty results through SearchFiltered.
type Options struct {
	Match        string // all (default) or any
	Scope        string // all (default), title, path or body
	Limit        int    // zero returns the full result set
	OmitSnippets bool   // Explorer has a separate selected-document preview
}

// Index is an in-memory full-text search index.
type Index struct {
	mu          sync.RWMutex
	docs, texts []Document
}

// SearchResult is one search hit.
type SearchResult struct {
	Path      string
	Title     string
	Snippet   string // Plain text with <mark> around matches
	Score     float64
	BookTitle string // Set by library mode to identify which book the result is from
	BookSlug  string // Set by library mode for badge coloring
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
	return &Index{}
}

// Add indexes a document.
func (idx *Index) Add(doc Document) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.docs = append(idx.docs, doc)
	idx.texts = append(idx.texts, Document{Path: fold(doc.Path), Title: fold(doc.Title), Body: fold(doc.Body)})
}

// extractHeadings returns H2/H3 headings from raw markdown source (max 4).
func extractHeadings(src string) []string {
	data := headingFrontmatter.ReplaceAll([]byte(src), nil)
	doc := goldmark.New().Parser().Parse(text.NewReader(data))
	var out []string
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if heading, ok := node.(*ast.Heading); ok && entering && (heading.Level == 2 || heading.Level == 3) {
			out = append(out, string(heading.Text(data)))
			if len(out) == 4 {
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
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
		if node.FSPath == "" || node.IsDir {
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
		idx.Add(Document{
			Path:     node.FullPath,
			Title:    node.Title,
			Body:     body,
			Headings: extractHeadings(string(src)),
		})
	})
	// Also index dir nodes that have an index.md (FSPath set on dir node)
	walkNodes(tree, func(node *navigation.NavNode) {
		if node.FSPath == "" || !node.IsDir {
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
		idx.Add(Document{
			Path:     node.FullPath,
			Title:    node.Title,
			Body:     body,
			Headings: extractHeadings(string(src)),
		})
	})
	return idx, nil
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
	if strings.TrimSpace(query) == "" {
		return nil
	}
	return idx.Query(query, Options{Match: "any", Limit: 20}, allowed)
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
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var clauses []string
	for _, match := range queryClause.FindAllStringSubmatch(query, -1) {
		text := match[1]
		if text == "" {
			text = match[2]
		}
		if text = fold(text); text != "" {
			clauses = append(clauses, text)
		}
	}
	var results []SearchResult
	for i, doc := range idx.docs {
		if allowed != nil && !allowed[doc.Path] {
			continue
		}
		text := idx.texts[i]
		matched, score := 0, 0.0
		for _, clause := range clauses {
			weight := 0.0
			if (options.Scope == "" || options.Scope == "all" || options.Scope == "body") && strings.Contains(text.Body, clause) {
				weight = 1
			}
			if (options.Scope == "" || options.Scope == "all" || options.Scope == "path") && strings.Contains(text.Path, clause) {
				weight += 3
			}
			if (options.Scope == "" || options.Scope == "all" || options.Scope == "title") && strings.Contains(text.Title, clause) {
				weight += 8
			}
			if weight > 0 {
				matched++
				score += weight
			}
		}
		if len(clauses) > 0 && ((options.Match == "any" && matched == 0) || (options.Match != "any" && matched != len(clauses))) {
			continue
		}
		results = append(results, SearchResult{
			Path:    doc.Path,
			Title:   doc.Title,
			Snippet: doc.Body,
			Score:   score,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Path < results[j].Path
		}
		return results[i].Score > results[j].Score
	})
	if options.Limit > 0 && len(results) > options.Limit {
		results = results[:options.Limit]
	}
	for i := range results {
		results[i].Title = highlightTerms(results[i].Title, clauses)
		if options.OmitSnippets {
			results[i].Snippet = ""
		} else {
			results[i].Snippet = extractSnippet(results[i].Snippet, clauses, 0)
		}
	}
	return results
}

// extractSnippet finds the first match in plain text and returns ±150 chars around it.
func extractSnippet(body string, tokens []string, _ int) string {
	runes := []rune(body)
	runePos := 0
	for i, matched := range matchRunes(body, tokens) {
		if matched {
			runePos = i
			break
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
			for i := positions[index]; i <= positions[index+len(term)-1]; i++ {
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
