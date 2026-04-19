package search

import (
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/mares/dyno/internal/navigation"
)

// Document is a single indexed page.
type Document struct {
	Path  string // URL path
	Title string
	Body  string // Plain text, for snippet extraction
}

// Posting records one occurrence of a token in a document.
type Posting struct {
	DocID    int
	Position int
}

// Index is an in-memory full-text search index.
type Index struct {
	mu      sync.RWMutex
	inverted map[string][]Posting
	docs    []Document
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

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "been": true, "but": true, "by": true, "for": true, "from": true,
	"has": true, "have": true, "he": true, "in": true, "is": true, "it": true,
	"its": true, "of": true, "on": true, "or": true, "that": true, "the": true,
	"this": true, "to": true, "was": true, "were": true, "will": true, "with": true,
}

var nonAlpha = regexp.MustCompile(`[^a-z0-9]+`)

func tokenize(text string) []string {
	lower := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, text)
	parts := nonAlpha.Split(lower, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) >= 2 && !stopWords[p] {
			out = append(out, p)
		}
	}
	return out
}

// New creates an empty index.
func New() *Index {
	return &Index{
		inverted: make(map[string][]Posting),
	}
}

// Add indexes a document.
func (idx *Index) Add(doc Document) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	docID := len(idx.docs)
	idx.docs = append(idx.docs, doc)

	tokens := tokenize(doc.Title + " " + doc.Body)
	for pos, tok := range tokens {
		idx.inverted[tok] = append(idx.inverted[tok], Posting{DocID: docID, Position: pos})
	}
}

// BuildIndex walks all .md files in the nav tree, renders them to plain text,
// and adds them to a new Index.
func BuildIndex(tree *navigation.NavNode, plainText func(path string) (string, error)) (*Index, error) {
	idx := New()
	walkNodes(tree, func(node *navigation.NavNode) {
		if node.FSPath == "" || node.IsDir {
			return
		}
		src, err := os.ReadFile(node.FSPath)
		if err != nil {
			return
		}
		body, err := plainText(string(src))
		if err != nil {
			return
		}
		idx.Add(Document{
			Path:  node.FullPath,
			Title: node.Title,
			Body:  body,
		})
	})
	// Also index dir nodes that have an index.md (FSPath set on dir node)
	walkNodes(tree, func(node *navigation.NavNode) {
		if node.FSPath == "" || !node.IsDir {
			return
		}
		src, err := os.ReadFile(node.FSPath)
		if err != nil {
			return
		}
		body, err := plainText(string(src))
		if err != nil {
			return
		}
		idx.Add(Document{
			Path:  node.FullPath,
			Title: node.Title,
			Body:  body,
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

// Search returns the top results for a query string.
func (idx *Index) Search(query string) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	tokens := tokenize(query)
	if len(tokens) == 0 {
		return nil
	}

	// Gather doc scores: docID → count of matching token occurrences.
	// For each query token, first try exact match, then prefix/substring match
	// against all indexed terms so that e.g. "config" finds "configuration".
	scores := make(map[int]float64)
	firstPos := make(map[int]int) // docID → position of first hit

	for _, tok := range tokens {
		// Exact match scores 1.0 per posting
		for _, p := range idx.inverted[tok] {
			scores[p.DocID] += 1.0
			if existing, ok := firstPos[p.DocID]; !ok || p.Position < existing {
				firstPos[p.DocID] = p.Position
			}
		}
		// Substring match on other indexed terms scores 0.5 (partial credit)
		for term, postings := range idx.inverted {
			if term == tok {
				continue // already counted
			}
			if strings.Contains(term, tok) {
				for _, p := range postings {
					scores[p.DocID] += 0.5
					if existing, ok := firstPos[p.DocID]; !ok || p.Position < existing {
						firstPos[p.DocID] = p.Position
					}
				}
			}
		}
	}

	// TF normalization
	results := make([]SearchResult, 0, len(scores))
	for docID, score := range scores {
		doc := idx.docs[docID]
		bodyLen := float64(len(tokenize(doc.Body)) + 1)
		normalized := score / math.Log(bodyLen+1)

		snippet := extractSnippet(doc.Body, tokens, firstPos[docID])
		results = append(results, SearchResult{
			Path:    doc.Path,
			Title:   highlightTerms(doc.Title, tokens),
			Snippet: snippet,
			Score:   normalized,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > 20 {
		results = results[:20]
	}
	return results
}

// extractSnippet finds the first match in plain text and returns ±150 chars around it.
func extractSnippet(body string, tokens []string, _ int) string {
	bodyLower := strings.ToLower(body)
	runes := []rune(body)

	// Find character position of first matching token
	charPos := 0
	for _, tok := range tokens {
		if idx := strings.Index(bodyLower, tok); idx >= 0 {
			charPos = idx
			break
		}
	}

	// Convert to rune position
	runePos := len([]rune(body[:charPos]))

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
	if len(tokens) == 0 {
		return text
	}
	// Match the token as a substring within a word (case-insensitive)
	escaped := make([]string, len(tokens))
	for i, t := range tokens {
		escaped[i] = regexp.QuoteMeta(t)
	}
	pattern := "(?i)(" + strings.Join(escaped, "|") + ")"
	re, err := regexp.Compile(pattern)
	if err != nil {
		return text
	}
	return re.ReplaceAllString(text, "<mark>$1</mark>")
}
