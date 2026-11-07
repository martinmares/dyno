package search

import (
	"log/slog"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blevesearch/bleve/v2"
	_ "github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	_ "github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	_ "github.com/blevesearch/bleve/v2/analysis/tokenizer/whitespace"
	"github.com/blevesearch/bleve/v2/search/query"
)

func newEngine() (bleve.Index, error) {
	mapping := bleve.NewIndexMapping()
	mapping.ScoringModel = "bm25"
	for name, tokenizer := range map[string]string{"words": "unicode", "literal": "whitespace"} {
		if err := mapping.AddCustomAnalyzer(name, map[string]interface{}{"type": "custom", "tokenizer": tokenizer}); err != nil {
			return nil, err
		}
	}
	doc := bleve.NewDocumentMapping()
	doc.Dynamic = false
	for _, name := range []string{"title", "path", "body", "heading"} {
		for _, suffix := range []string{"", "_literal"} {
			field := bleve.NewTextFieldMapping()
			field.Analyzer = "words"
			if suffix != "" {
				field.Analyzer = "literal"
			}
			field.Store = false
			field.IncludeInAll = false
			doc.AddFieldMappingsAt(name+suffix, field)
		}
	}
	mapping.DefaultMapping = doc
	return bleve.NewMemOnly(mapping)
}

// Preserve identifiers as well as indexing their component words. In particular,
// REQ0129276 must not become just REQ, and release/RE63 must remain distinguishable.
var literalWords = regexp.MustCompile(`[\pL\pN]+(?:[-_./][\pL\pN]+)*`)

func engineDocument(doc Document) map[string]string {
	headings := strings.Join(doc.Headings, " ")
	if len(doc.Sections) > 0 {
		headings = ""
	}
	for _, section := range doc.Sections {
		headings += " " + section.Heading
	}
	fields := map[string]string{"title": fold(doc.Title), "path": fold(doc.Path), "body": fold(doc.Body), "heading": fold(headings)}
	for _, name := range []string{"title", "path", "body", "heading"} {
		fields[name+"_literal"] = strings.Join(literalWords.FindAllString(fields[name], -1), " ")
	}
	// URL segments must also be searchable by an identifier or filename without
	// requiring the entire directory prefix in Explorer's path-only scope.
	fields["path_literal"] += " " + strings.Join(strings.Split(strings.Trim(fields["path"], "/"), "/"), " ")
	return fields
}

type clause struct {
	text   string
	phrase bool
}

func parseQuery(input string) []clause {
	var clauses []clause
	for _, match := range queryClause.FindAllStringSubmatch(input, -1) {
		value := match[2]
		if match[1] != "" {
			value = match[1]
		} else {
			value = strings.Trim(value, "`,.;:!?()[]{}")
		}
		if value = fold(value); value != "" {
			clauses = append(clauses, clause{value, match[1] != ""})
		}
		// Bound Boolean expansion for untrusted, arbitrarily long URL queries.
		if len(clauses) == 32 {
			break
		}
	}
	return clauses
}

func technical(s string) bool {
	return strings.ContainsAny(s, "-_/.") || strings.ContainsFunc(s, unicode.IsDigit)
}

func buildQuery(clauses []clause, options Options, fuzzy bool) query.Query {
	if len(clauses) == 0 {
		return bleve.NewMatchAllQuery()
	}
	fields := []struct {
		name  string
		boost float64
	}{{"title", 8}, {"heading", 5}, {"path", 3}, {"body", 1}}
	var required []query.Query
	for i, clause := range clauses {
		var alternatives []query.Query
		for _, field := range fields {
			if options.Scope != "" && options.Scope != "all" && field.name != options.Scope {
				continue
			}
			if clause.phrase {
				q := bleve.NewMatchPhraseQuery(clause.text)
				q.SetField(field.name)
				q.Analyzer = "words"
				q.SetBoost(field.boost)
				alternatives = append(alternatives, q)
				continue
			}
			name := field.name + "_literal"
			term := strings.Trim(clause.text, "/")
			if technical(term) {
				q := bleve.NewTermQuery(term)
				q.SetField(name)
				q.SetBoost(field.boost)
				alternatives = append(alternatives, q)
				if i == len(clauses)-1 && utf8.RuneCountInString(term) >= 3 {
					prefix := bleve.NewPrefixQuery(term)
					prefix.SetField(name)
					prefix.SetBoost(field.boost * .2)
					alternatives = append(alternatives, prefix)
				}
				continue
			}
			q := bleve.NewMatchQuery(term)
			q.SetField(field.name)
			q.Analyzer = "words"
			q.SetBoost(field.boost)
			alternatives = append(alternatives, q)
			if i == len(clauses)-1 && utf8.RuneCountInString(term) >= 2 {
				prefix := bleve.NewPrefixQuery(term)
				prefix.SetField(field.name)
				prefix.SetBoost(field.boost * .4)
				alternatives = append(alternatives, prefix)
			}
			if fuzzy && utf8.RuneCountInString(term) >= 5 && utf8.RuneCountInString(term) <= 40 {
				approx := bleve.NewFuzzyQuery(term)
				approx.SetField(field.name)
				approx.SetFuzziness(1)
				approx.SetPrefix(1)
				approx.SetBoost(field.boost * .2)
				alternatives = append(alternatives, approx)
			}
		}
		if len(alternatives) == 0 {
			return bleve.NewMatchNoneQuery()
		}
		required = append(required, bleve.NewDisjunctionQuery(alternatives...))
	}
	var base query.Query = bleve.NewConjunctionQuery(required...)
	if options.Match == "any" {
		base = bleve.NewDisjunctionQuery(required...)
	}
	if len(clauses) < 2 {
		return base
	}
	// Adjacent query words are a stronger signal than scattered mentions, but
	// this optional bonus must never change which documents satisfy all/any.
	boosted := bleve.NewBooleanQuery()
	boosted.AddMust(base)
	var words []string
	for _, clause := range clauses {
		words = append(words, clause.text)
	}
	for _, field := range fields {
		if options.Scope != "" && options.Scope != "all" && field.name != options.Scope {
			continue
		}
		phrase := bleve.NewMatchPhraseQuery(strings.Join(words, " "))
		phrase.SetField(field.name)
		phrase.Analyzer = "words"
		phrase.SetBoost(field.boost * 2)
		boosted.AddShould(phrase)
	}
	return boosted
}

func (idx *Index) search(input string, options Options, allowed map[string]bool) ([]SearchResult, int) {
	clauses := parseQuery(input)
	if strings.TrimSpace(input) != "" && len(clauses) == 0 {
		return nil, 0
	}
	byPath := make(map[string]Document, len(idx.docs))
	for _, doc := range idx.docs {
		byPath[doc.Path] = doc
	}
	var ids []string
	if allowed != nil {
		for path, ok := range allowed {
			if ok {
				ids = append(ids, path)
			}
		}
		if len(ids) == 0 {
			return nil, 0
		}
	}
	var results []SearchResult
	// Relax spelling only when no strict result exists, not merely when the
	// requested page is empty. Never silently relax quoted phrases or identifiers.
	for _, fuzzy := range []bool{false, true} {
		if fuzzy && (len(clauses) == 0 || len(results) > 0) {
			break
		}
		q := buildQuery(clauses, options, fuzzy)
		if allowed != nil {
			q = bleve.NewConjunctionQuery(q, bleve.NewDocIDQuery(ids))
		}
		size := options.Limit
		if size <= 0 {
			size = len(idx.docs)
		}
		offset := max(0, options.Offset)
		request := bleve.NewSearchRequestOptions(q, size, offset, false)
		request.IncludeLocations = !options.OmitSnippets
		request.SortBy([]string{"-_score", "_id"})
		hits, err := idx.engine.Search(request)
		if err != nil {
			slog.Error("search failed", "err", err)
			return nil, 0
		}
		if hits.Total == 0 {
			continue
		}
		for _, hit := range hits.Hits {
			doc := byPath[hit.ID]
			var terms []string
			for _, c := range clauses {
				terms = append(terms, c.text)
			}
			for _, locations := range hit.Locations {
				for term := range locations {
					terms = append(terms, term)
				}
			}
			terms = uniqueTerms(terms)
			result := SearchResult{Path: doc.Path, Title: highlightTerms(doc.Title, terms), Score: hit.Score, Approximate: fuzzy}
			if !options.OmitSnippets {
				section := bestSection(doc, terms)
				result.Section, result.Anchor = section.Title, section.Anchor
				result.Snippet = extractSnippet(section.Body, terms, 0)
			}
			results = append(results, result)
		}
		return results, int(hits.Total)
	}
	return results, 0
}

func uniqueTerms(terms []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, term := range terms {
		if term != "" && !seen[term] {
			seen[term] = true
			out = append(out, term)
		}
	}
	return out
}
