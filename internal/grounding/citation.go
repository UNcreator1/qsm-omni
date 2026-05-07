package grounding

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
)

type Citation struct {
	ID         string  `json:"id"`
	Source     string  `json:"source"`
	SourceType string  `json:"source_type,omitempty"`
	SentenceID int     `json:"sentence_id,omitempty"`
	Quote      string  `json:"quote"`
	Score      float64 `json:"score"`
}

type CitationReport struct {
	Citations []Citation `json:"citations,omitempty"`
	Coverage  float64    `json:"coverage,omitempty"`
	Missing   []string   `json:"missing,omitempty"`
}

type Candidate struct {
	Source     string
	SourceType string
	Text       string
}

type Query struct {
	Text   string
	Source string
}

func MapQueries(candidates []Candidate, queries []Query, limit int) CitationReport {
	if limit <= 0 {
		limit = 5
	}
	passages := splitCandidates(candidates)
	var citations []Citation
	matched := map[int]bool{}
	for queryIndex, query := range queries {
		queryText := strings.TrimSpace(query.Text)
		if queryText == "" {
			continue
		}
		best, ok := bestCitation(passages, queryText)
		if !ok {
			continue
		}
		matched[queryIndex] = true
		citations = append(citations, best)
	}
	citations = dedupeCitations(citations)
	sort.SliceStable(citations, func(i, j int) bool {
		if citations[i].Score == citations[j].Score {
			return citations[i].ID < citations[j].ID
		}
		return citations[i].Score > citations[j].Score
	})
	if len(citations) > limit {
		citations = citations[:limit]
	}
	report := CitationReport{Citations: citations}
	if len(queries) > 0 {
		report.Coverage = float64(len(matched)) / float64(len(queries))
		for index, query := range queries {
			if !matched[index] && strings.TrimSpace(query.Text) != "" {
				label := query.Source
				if label == "" {
					label = truncate(strings.TrimSpace(query.Text), 80)
				}
				report.Missing = append(report.Missing, label)
			}
		}
	}
	return report
}

func CandidatesFromLake(q *lake.Lake, objectiveID string) []Candidate {
	if q == nil {
		return nil
	}
	var out []Candidate
	artifacts, err := q.List()
	if err == nil {
		for _, artifact := range artifacts {
			text := strings.TrimSpace(strings.Join([]string{artifact.Claim, artifact.Content}, "\n\n"))
			if text == "" {
				continue
			}
			out = append(out, Candidate{
				Source:     filepath.Join(q.Root(), "artifacts", artifact.ID+".json"),
				SourceType: "lake_artifact",
				Text:       text,
			})
		}
	}
	verified := true
	items, err := q.ListCache(lake.CacheFilter{ObjectiveID: objectiveID, Verified: &verified})
	if err == nil {
		for _, item := range items {
			if strings.TrimSpace(item.Content) == "" {
				continue
			}
			out = append(out, Candidate{
				Source:     "cache_item:" + item.ID,
				SourceType: "cache_item",
				Text:       item.Content,
			})
		}
	}
	return out
}

func CandidateFromFile(path, sourceType string) (Candidate, bool) {
	data, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return Candidate{}, false
	}
	return Candidate{Source: path, SourceType: sourceType, Text: string(data)}, true
}

type passage struct {
	source     string
	sourceType string
	sentenceID int
	text       string
	tokens     map[string]int
}

func splitCandidates(candidates []Candidate) []passage {
	var out []passage
	for _, candidate := range candidates {
		parts := splitPassages(candidate.Text)
		for i, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			out = append(out, passage{
				source:     candidate.Source,
				sourceType: candidate.SourceType,
				sentenceID: i + 1,
				text:       part,
				tokens:     tokenCounts(part),
			})
		}
	}
	return out
}

var passageSplit = regexp.MustCompile(`(\n\s*\n+|[.!?]\s+)`)

func splitPassages(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	raw := passageSplit.Split(text, -1)
	var out []string
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if len(part) > 700 {
			out = append(out, splitLongPassage(part, 700)...)
			continue
		}
		out = append(out, part)
	}
	return out
}

func splitLongPassage(text string, limit int) []string {
	words := strings.Fields(text)
	var out []string
	var b strings.Builder
	for _, word := range words {
		if b.Len() > 0 && b.Len()+1+len(word) > limit {
			out = append(out, b.String())
			b.Reset()
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(word)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func bestCitation(passages []passage, query string) (Citation, bool) {
	queryTokens := tokenCounts(query)
	if len(queryTokens) == 0 {
		return Citation{}, false
	}
	var best passage
	bestScore := 0.0
	for _, candidate := range passages {
		score := overlapScore(query, queryTokens, candidate.text, candidate.tokens)
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}
	if bestScore < 0.28 {
		return Citation{}, false
	}
	id := citationID(best.source, best.sentenceID, best.text)
	return Citation{
		ID:         id,
		Source:     best.source,
		SourceType: best.sourceType,
		SentenceID: best.sentenceID,
		Quote:      best.text,
		Score:      roundScore(bestScore),
	}, true
}

func overlapScore(query string, queryTokens map[string]int, text string, textTokens map[string]int) float64 {
	if len(queryTokens) == 0 || len(textTokens) == 0 {
		return 0
	}
	overlap := 0
	total := 0
	for token, count := range queryTokens {
		total += count
		if textCount := textTokens[token]; textCount > 0 {
			if count < textCount {
				overlap += count
			} else {
				overlap += textCount
			}
		}
	}
	score := float64(overlap) / float64(total)
	q := strings.ToLower(strings.TrimSpace(query))
	t := strings.ToLower(strings.TrimSpace(text))
	if len(q) >= 12 && strings.Contains(t, q) {
		score += 0.25
	}
	if score > 1 {
		return 1
	}
	return score
}

func tokenCounts(text string) map[string]int {
	counts := map[string]int{}
	for _, token := range tokenize(text) {
		counts[token]++
	}
	return counts
}

func tokenize(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
}

func dedupeCitations(citations []Citation) []Citation {
	seen := map[string]Citation{}
	for _, citation := range citations {
		existing, ok := seen[citation.ID]
		if !ok || citation.Score > existing.Score {
			seen[citation.ID] = citation
		}
	}
	out := make([]Citation, 0, len(seen))
	for _, citation := range seen {
		out = append(out, citation)
	}
	return out
}

func citationID(source string, sentenceID int, quote string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", source, sentenceID, quote)))
	return hex.EncodeToString(sum[:])[:16]
}

func roundScore(score float64) float64 {
	return float64(int(score*1000+0.5)) / 1000
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "...[truncated]"
}
