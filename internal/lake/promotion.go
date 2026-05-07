package lake

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const PromotionSchema = "qsm.lake_promotion_report.v1"

type PromotionPolicy struct {
	Apply         bool    `json:"apply"`
	MinRepeat     int     `json:"min_repeat"`
	MinConfidence float64 `json:"min_confidence"`
	MaxPromotions int     `json:"max_promotions"`
}

type PromotionReport struct {
	Schema           string             `json:"schema"`
	CreatedAt        time.Time          `json:"created_at"`
	Apply            bool               `json:"apply"`
	Policy           PromotionPolicy    `json:"policy"`
	Reviewed         int                `json:"reviewed"`
	Promoted         int                `json:"promoted"`
	Rejected         int                `json:"rejected"`
	Promotions       []CuratedPromotion `json:"promotions,omitempty"`
	Rejections       []PromotionReject  `json:"rejections,omitempty"`
	ArtifactsWritten []string           `json:"artifacts_written,omitempty"`
	CuratedFiles     []string           `json:"curated_files,omitempty"`
	Recommendations  []string           `json:"recommendations,omitempty"`
}

type CuratedPromotion struct {
	Title      string   `json:"title"`
	Kind       string   `json:"kind"`
	Count      int      `json:"count"`
	Confidence float64  `json:"confidence"`
	CacheIDs   []string `json:"cache_ids"`
	Content    string   `json:"content"`
	Rationale  string   `json:"rationale"`
}

type PromotionReject struct {
	Content    string  `json:"content"`
	Count      int     `json:"count"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

func DefaultPromotionPolicy() PromotionPolicy {
	return PromotionPolicy{
		MinRepeat:     3,
		MinConfidence: 0.75,
		MaxPromotions: 12,
	}
}

func (l *Lake) PromoteCache(policy PromotionPolicy) (PromotionReport, error) {
	if policy.MinRepeat <= 0 {
		policy.MinRepeat = DefaultPromotionPolicy().MinRepeat
	}
	if policy.MinConfidence <= 0 {
		policy.MinConfidence = DefaultPromotionPolicy().MinConfidence
	}
	if policy.MaxPromotions <= 0 {
		policy.MaxPromotions = DefaultPromotionPolicy().MaxPromotions
	}
	files, err := l.listCacheFiles()
	if err != nil {
		return PromotionReport{}, err
	}
	candidates := promotionCandidates(files)
	report := PromotionReport{
		Schema:    PromotionSchema,
		CreatedAt: time.Now().UTC(),
		Apply:     policy.Apply,
		Policy:    policy,
		Reviewed:  len(candidates),
	}
	for _, candidate := range candidates {
		promotion, reject := classifyPromotion(candidate, policy)
		if reject.Reason != "" {
			report.Rejections = append(report.Rejections, reject)
			continue
		}
		report.Promotions = append(report.Promotions, promotion)
	}
	sort.Slice(report.Promotions, func(i, j int) bool {
		if report.Promotions[i].Count == report.Promotions[j].Count {
			return report.Promotions[i].Confidence > report.Promotions[j].Confidence
		}
		return report.Promotions[i].Count > report.Promotions[j].Count
	})
	if len(report.Promotions) > policy.MaxPromotions {
		for _, extra := range report.Promotions[policy.MaxPromotions:] {
			report.Rejections = append(report.Rejections, PromotionReject{
				Content:    extra.Content,
				Count:      extra.Count,
				Confidence: extra.Confidence,
				Reason:     "above_max_promotions_limit",
			})
		}
		report.Promotions = report.Promotions[:policy.MaxPromotions]
	}
	report.Promoted = len(report.Promotions)
	report.Rejected = len(report.Rejections)
	if policy.Apply && report.Promoted > 0 {
		if err := l.writeCuratedPromotions(&report); err != nil {
			return report, err
		}
	}
	if report.Promoted == 0 {
		report.Recommendations = append(report.Recommendations, "No high-signal repeated recipes passed promotion gates; keep collecting better evidence.")
	}
	if report.Rejected > 0 {
		report.Recommendations = append(report.Recommendations, "Rejected repeated low-signal operational messages; improve node evidence to write reusable problem/solution recipes.")
	}
	if !policy.Apply && report.Promoted > 0 {
		report.Recommendations = append(report.Recommendations, "Run qsm lake-promote -apply=true to write curated best practices into .lake/curated and artifacts.")
	}
	return report, nil
}

func WritePromotionReport(root string, report PromotionReport) error {
	stateDir := filepath.Join(root, ".state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stateDir, "lake_promotion_report.json"), data, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, "lake_promotion_report.md"), []byte(PromotionMarkdown(report)), 0644)
}

func PromotionMarkdown(report PromotionReport) string {
	var b strings.Builder
	b.WriteString("# QSM Lake Promotion Report\n\n")
	fmt.Fprintf(&b, "- Apply: `%v`\n", report.Apply)
	fmt.Fprintf(&b, "- Reviewed candidates: `%d`\n", report.Reviewed)
	fmt.Fprintf(&b, "- Promoted: `%d`\n", report.Promoted)
	fmt.Fprintf(&b, "- Rejected: `%d`\n", report.Rejected)
	if len(report.Promotions) > 0 {
		b.WriteString("\n## Curated Promotions\n\n")
		b.WriteString("| Title | Count | Confidence | Rationale |\n")
		b.WriteString("| --- | ---: | ---: | --- |\n")
		for _, item := range report.Promotions {
			fmt.Fprintf(&b, "| %s | %d | %.2f | %s |\n", escapePipe(item.Title), item.Count, item.Confidence, escapePipe(item.Rationale))
		}
	}
	if len(report.Rejections) > 0 {
		b.WriteString("\n## Rejections\n\n")
		b.WriteString("| Count | Confidence | Reason | Content |\n")
		b.WriteString("| ---: | ---: | --- | --- |\n")
		for _, item := range report.Rejections {
			content := item.Content
			if len(content) > 120 {
				content = content[:120] + "..."
			}
			fmt.Fprintf(&b, "| %d | %.2f | %s | %s |\n", item.Count, item.Confidence, item.Reason, escapePipe(content))
		}
	}
	if len(report.CuratedFiles) > 0 {
		b.WriteString("\n## Curated Files\n\n")
		for _, path := range report.CuratedFiles {
			b.WriteString("- `" + path + "`\n")
		}
	}
	if len(report.Recommendations) > 0 {
		b.WriteString("\n## Recommendations\n\n")
		for _, rec := range report.Recommendations {
			b.WriteString("- " + rec + "\n")
		}
	}
	return b.String()
}

func classifyPromotion(candidate PromotionCandidate, policy PromotionPolicy) (CuratedPromotion, PromotionReject) {
	if candidate.Count < policy.MinRepeat {
		return CuratedPromotion{}, PromotionReject{Content: candidate.Content, Count: candidate.Count, Confidence: candidate.Confidence, Reason: "below_min_repeat"}
	}
	if candidate.Confidence < policy.MinConfidence {
		return CuratedPromotion{}, PromotionReject{Content: candidate.Content, Count: candidate.Count, Confidence: candidate.Confidence, Reason: "below_min_confidence"}
	}
	if lowSignalRecipe(candidate.Content) {
		return CuratedPromotion{}, PromotionReject{Content: candidate.Content, Count: candidate.Count, Confidence: candidate.Confidence, Reason: "low_signal_operational_message"}
	}
	title := promotionTitle(candidate.Content)
	return CuratedPromotion{
		Title:      title,
		Kind:       candidate.Kind,
		Count:      candidate.Count,
		Confidence: candidate.Confidence,
		CacheIDs:   append([]string(nil), candidate.CacheIDs...),
		Content:    candidate.Content,
		Rationale:  fmt.Sprintf("Repeated %d times with average confidence %.2f and passed high-signal filters.", candidate.Count, candidate.Confidence),
	}, PromotionReject{}
}

func (l *Lake) writeCuratedPromotions(report *PromotionReport) error {
	dir := filepath.Join(l.root, "curated")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	jsonPath := filepath.Join(dir, "best_practices.json")
	mdPath := filepath.Join(dir, "best_practices.md")
	data, err := json.MarshalIndent(report.Promotions, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(mdPath, []byte(curatedMarkdown(report.Promotions)), 0644); err != nil {
		return err
	}
	report.CuratedFiles = append(report.CuratedFiles, jsonPath, mdPath)
	for _, promotion := range report.Promotions {
		artifact, err := l.Put(Artifact{
			Phase:      PhaseResearch,
			Kind:       "curated_best_practice",
			Source:     "qsm-lake-promote",
			Claim:      promotion.Title,
			Content:    promotion.Content,
			Confidence: promotion.Confidence,
			Verified:   true,
			Metadata: map[string]string{
				"cache_ids": strings.Join(promotion.CacheIDs, ","),
				"count":     fmt.Sprint(promotion.Count),
				"rationale": promotion.Rationale,
			},
		})
		if err != nil {
			return err
		}
		report.ArtifactsWritten = append(report.ArtifactsWritten, artifact.ID)
	}
	return nil
}

func curatedMarkdown(promotions []CuratedPromotion) string {
	var b strings.Builder
	b.WriteString("# QSM Curated Best Practices\n\n")
	b.WriteString("These items were promoted from repeated verified cache recipes. Treat them as stronger than raw cache, but keep provenance attached.\n\n")
	for _, item := range promotions {
		fmt.Fprintf(&b, "## %s\n\n", item.Title)
		fmt.Fprintf(&b, "- Count: `%d`\n", item.Count)
		fmt.Fprintf(&b, "- Confidence: `%.2f`\n", item.Confidence)
		fmt.Fprintf(&b, "- Cache IDs: `%s`\n", strings.Join(item.CacheIDs, ", "))
		fmt.Fprintf(&b, "- Rationale: %s\n\n", item.Rationale)
		b.WriteString("```text\n")
		b.WriteString(item.Content)
		b.WriteString("\n```\n\n")
	}
	return b.String()
}

func lowSignalRecipe(content string) bool {
	text := strings.ToLower(strings.Join(strings.Fields(content), " "))
	if len(text) < 80 {
		return true
	}
	patterns := []string{
		"product accepted by qsm streaming stop policy",
		"deepagents node completed with stop_reason",
		"product directory is non-empty",
		"index.html has an html document",
		"static web product has interactive/script surface",
		"qsm test commands passed:",
	}
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	words := strings.Fields(text)
	unique := map[string]bool{}
	for _, word := range words {
		unique[word] = true
	}
	return len(unique) < 10
}

func promotionTitle(content string) string {
	words := strings.Fields(content)
	if len(words) > 10 {
		words = words[:10]
	}
	title := strings.Trim(strings.Join(words, " "), ". ")
	if title == "" {
		return "Curated best practice"
	}
	return strings.ToUpper(title[:1]) + title[1:]
}

func escapePipe(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}
