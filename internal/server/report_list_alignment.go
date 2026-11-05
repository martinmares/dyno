package server

import (
	"regexp"
	"strings"
)

var reportListWords = regexp.MustCompile(`[\p{L}\p{M}\p{N}_]+`)

type reportListContent struct {
	words   map[string]int
	total   int
	section string
}

func listContent(text, section string) reportListContent {
	content := reportListContent{words: map[string]int{}, section: section}
	for _, word := range reportListWords.FindAllString(strings.ToLower(text), -1) {
		content.words[word]++
		content.total++
	}
	return content
}

// Compare visible words, not bullets, numbering or link destinations. A pair
// must share at least half its words; prefer a strong match over weak matches.
func reportListSimilarity(a, b reportListContent) int {
	if a.section != b.section || a.total+b.total == 0 {
		return 0
	}
	common := 0
	for word, count := range a.words {
		common += min(count, b.words[word])
	}
	score := 200 * common / (a.total + b.total)
	if score < 50 {
		return 0
	}
	return score - 49
}

func alignReportLists(rows []gitCompareRow, before, after []markdownReportBlock, plainText func([]byte) (string, error)) ([]gitCompareRow, error) {
	isList := func(row gitCompareRow) bool {
		return row.Changed && (row.LeftLine == 0 || before[row.LeftLine-1].Kind == "List") && (row.RightLine == 0 || after[row.RightLine-1].Kind == "List")
	}
	var result []gitCompareRow
	for start := 0; start < len(rows); {
		if !isList(rows[start]) {
			result = append(result, rows[start])
			start++
			continue
		}
		end := start
		var left, right []int
		for end < len(rows) && isList(rows[end]) {
			if rows[end].LeftLine > 0 {
				left = append(left, rows[end].LeftLine)
			}
			if rows[end].RightLine > 0 {
				right = append(right, rows[end].RightLine)
			}
			end++
		}
		appendRow := func(old, new int) {
			kind := "change"
			if old == 0 {
				kind = "add"
			} else if new == 0 {
				kind = "delete"
			}
			result = append(result, gitCompareRow{LeftLine: old, RightLine: new, Kind: kind, Changed: true})
		}
		// Large rewrites stay conservative rather than allocating an unbounded matrix.
		if len(left)*len(right) > 250_000 {
			for _, line := range left {
				appendRow(line, 0)
			}
			for _, line := range right {
				appendRow(0, line)
			}
			start = end
			continue
		}
		contents := func(indices []int, blocks []markdownReportBlock) ([]reportListContent, error) {
			var values []reportListContent
			for _, line := range indices {
				block := blocks[line-1]
				text, err := plainText([]byte(block.Source))
				if err != nil {
					return nil, err
				}
				values = append(values, listContent(text, block.Section))
			}
			return values, nil
		}
		a, err := contents(left, before)
		if err != nil {
			return nil, err
		}
		b, err := contents(right, after)
		if err != nil {
			return nil, err
		}
		width := len(right) + 1
		dp := make([]int, (len(left)+1)*width)
		scores := make([]int, len(left)*len(right))
		for i := len(left) - 1; i >= 0; i-- {
			for j := len(right) - 1; j >= 0; j-- {
				score := reportListSimilarity(a[i], b[j])
				scores[i*len(right)+j] = score
				dp[i*width+j] = max(dp[(i+1)*width+j], dp[i*width+j+1])
				if score > 0 {
					dp[i*width+j] = max(dp[i*width+j], score+dp[(i+1)*width+j+1])
				}
			}
		}
		for i, j := 0, 0; i < len(left) || j < len(right); {
			if i < len(left) && j < len(right) && scores[i*len(right)+j] > 0 && dp[i*width+j] == scores[i*len(right)+j]+dp[(i+1)*width+j+1] {
				appendRow(left[i], right[j])
				i++
				j++
			} else if j == len(right) || (i < len(left) && dp[(i+1)*width+j] >= dp[i*width+j+1]) {
				appendRow(left[i], 0)
				i++
			} else {
				appendRow(0, right[j])
				j++
			}
		}
		start = end
	}
	return result, nil
}
