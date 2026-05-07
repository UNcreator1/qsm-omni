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

const MaintenanceSchema = "qsm.lake_maintenance_report.v1"

type MaintenancePolicy struct {
	Apply                 bool          `json:"apply"`
	StaleAfter            time.Duration `json:"stale_after"`
	RouteHealthStaleAfter time.Duration `json:"route_health_stale_after"`
	MinConfidence         float64       `json:"min_confidence"`
	MaxRankedItems        int           `json:"max_ranked_items"`
}

type MaintenanceReport struct {
	Schema               string                `json:"schema"`
	CreatedAt            time.Time             `json:"created_at"`
	Apply                bool                  `json:"apply"`
	Policy               MaintenancePolicy     `json:"policy"`
	TotalCacheItems      int                   `json:"total_cache_items"`
	KeptCacheItems       int                   `json:"kept_cache_items"`
	QuarantineCount      int                   `json:"quarantine_count"`
	KindSummary          map[string]int        `json:"kind_summary"`
	ObjectiveSummary     map[string]int        `json:"objective_summary"`
	QuarantineCandidates []QuarantineCandidate `json:"quarantine_candidates,omitempty"`
	PromotionCandidates  []PromotionCandidate  `json:"promotion_candidates,omitempty"`
	Actions              []string              `json:"actions,omitempty"`
	Recommendations      []string              `json:"recommendations,omitempty"`
}

type QuarantineCandidate struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	ObjectiveID    string    `json:"objective_id,omitempty"`
	Producer       string    `json:"producer,omitempty"`
	Reason         string    `json:"reason"`
	Confidence     float64   `json:"confidence"`
	CreatedAt      time.Time `json:"created_at"`
	Path           string    `json:"path"`
	QuarantinePath string    `json:"quarantine_path,omitempty"`
}

type PromotionCandidate struct {
	Kind       string   `json:"kind"`
	Count      int      `json:"count"`
	Confidence float64  `json:"confidence"`
	CacheIDs   []string `json:"cache_ids"`
	Content    string   `json:"content"`
	Reason     string   `json:"reason"`
}

type cacheFileItem struct {
	Item CacheItem
	Path string
}

func DefaultMaintenancePolicy() MaintenancePolicy {
	return MaintenancePolicy{
		StaleAfter:            30 * 24 * time.Hour,
		RouteHealthStaleAfter: 24 * time.Hour,
		MinConfidence:         0.45,
		MaxRankedItems:        30,
	}
}

func (l *Lake) MaintainCache(policy MaintenancePolicy) (MaintenanceReport, error) {
	if policy.StaleAfter <= 0 {
		policy.StaleAfter = DefaultMaintenancePolicy().StaleAfter
	}
	if policy.RouteHealthStaleAfter <= 0 {
		policy.RouteHealthStaleAfter = DefaultMaintenancePolicy().RouteHealthStaleAfter
	}
	if policy.MinConfidence <= 0 {
		policy.MinConfidence = DefaultMaintenancePolicy().MinConfidence
	}
	if policy.MaxRankedItems <= 0 {
		policy.MaxRankedItems = DefaultMaintenancePolicy().MaxRankedItems
	}
	now := time.Now().UTC()
	files, err := l.listCacheFiles()
	if err != nil {
		return MaintenanceReport{}, err
	}
	report := MaintenanceReport{
		Schema:           MaintenanceSchema,
		CreatedAt:        now,
		Apply:            policy.Apply,
		Policy:           policy,
		TotalCacheItems:  len(files),
		KindSummary:      map[string]int{},
		ObjectiveSummary: map[string]int{},
	}
	for _, file := range files {
		report.KindSummary[file.Item.Kind]++
		objective := file.Item.ObjectiveID
		if objective == "" {
			objective = "global"
		}
		report.ObjectiveSummary[objective]++
	}
	report.QuarantineCandidates = quarantineCandidates(files, policy, now)
	report.PromotionCandidates = promotionCandidates(files)
	report.QuarantineCount = len(report.QuarantineCandidates)
	report.KeptCacheItems = report.TotalCacheItems - report.QuarantineCount
	if policy.Apply {
		for i, candidate := range report.QuarantineCandidates {
			dst, err := l.quarantineCacheFile(candidate.Path, candidate.Reason, now)
			if err != nil {
				return report, err
			}
			report.QuarantineCandidates[i].QuarantinePath = dst
			report.Actions = append(report.Actions, fmt.Sprintf("quarantined %s reason=%s", candidate.ID, candidate.Reason))
		}
	} else if report.QuarantineCount > 0 {
		report.Recommendations = append(report.Recommendations, "Run qsm lake-maintain -apply=true to move prune candidates into .lake/quarantine/cache.")
	}
	if len(report.PromotionCandidates) > 0 {
		report.Recommendations = append(report.Recommendations, "Promote repeated high-confidence recipes into wiki/materials planning guidance after human review.")
	}
	if report.TotalCacheItems > 0 && report.QuarantineCount == 0 {
		report.Recommendations = append(report.Recommendations, "No cache quarantine candidates found under current policy.")
	}
	return report, nil
}

func WriteMaintenanceReport(root string, report MaintenanceReport) error {
	stateDir := filepath.Join(root, ".state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stateDir, "lake_maintenance_report.json"), data, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, "lake_maintenance_report.md"), []byte(MaintenanceMarkdown(report)), 0644)
}

func MaintenanceMarkdown(report MaintenanceReport) string {
	var b strings.Builder
	b.WriteString("# QSM Lake Maintenance Report\n\n")
	fmt.Fprintf(&b, "- Apply: `%v`\n", report.Apply)
	fmt.Fprintf(&b, "- Total cache items: `%d`\n", report.TotalCacheItems)
	fmt.Fprintf(&b, "- Kept cache items: `%d`\n", report.KeptCacheItems)
	fmt.Fprintf(&b, "- Quarantine candidates: `%d`\n", report.QuarantineCount)
	fmt.Fprintf(&b, "- Promotion candidates: `%d`\n", len(report.PromotionCandidates))
	b.WriteString("\n## Kind Summary\n\n")
	for _, key := range sortedMapKeys(report.KindSummary) {
		fmt.Fprintf(&b, "- `%s`: `%d`\n", key, report.KindSummary[key])
	}
	if len(report.QuarantineCandidates) > 0 {
		b.WriteString("\n## Quarantine Candidates\n\n")
		b.WriteString("| ID | Kind | Objective | Reason | Confidence | Created |\n")
		b.WriteString("| --- | --- | --- | --- | ---: | --- |\n")
		for _, item := range report.QuarantineCandidates {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %.2f | %s |\n", item.ID, item.Kind, item.ObjectiveID, item.Reason, item.Confidence, item.CreatedAt.Format(time.RFC3339))
		}
	}
	if len(report.PromotionCandidates) > 0 {
		b.WriteString("\n## Promotion Candidates\n\n")
		b.WriteString("| Kind | Count | Confidence | Reason | Content |\n")
		b.WriteString("| --- | ---: | ---: | --- | --- |\n")
		for _, item := range report.PromotionCandidates {
			content := strings.ReplaceAll(item.Content, "|", "\\|")
			if len(content) > 120 {
				content = content[:120] + "..."
			}
			fmt.Fprintf(&b, "| %s | %d | %.2f | %s | %s |\n", item.Kind, item.Count, item.Confidence, item.Reason, content)
		}
	}
	if len(report.Actions) > 0 {
		b.WriteString("\n## Actions\n\n")
		for _, action := range report.Actions {
			b.WriteString("- " + action + "\n")
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

func (l *Lake) listCacheFiles() ([]cacheFileItem, error) {
	entries, err := os.ReadDir(l.cacheDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []cacheFileItem
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(l.cacheDir(), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var item CacheItem
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		out = append(out, cacheFileItem{Item: item, Path: path})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Item.CreatedAt.Equal(out[j].Item.CreatedAt) {
			return out[i].Item.ID < out[j].Item.ID
		}
		return out[i].Item.CreatedAt.Before(out[j].Item.CreatedAt)
	})
	return out, nil
}

func quarantineCandidates(files []cacheFileItem, policy MaintenancePolicy, now time.Time) []QuarantineCandidate {
	best := map[string]cacheFileItem{}
	for _, file := range files {
		key := cacheDedupeKey(file.Item)
		prev, ok := best[key]
		if !ok || cacheRank(file.Item) > cacheRank(prev.Item) || (cacheRank(file.Item) == cacheRank(prev.Item) && file.Item.CreatedAt.After(prev.Item.CreatedAt)) {
			best[key] = file
		}
	}
	var out []QuarantineCandidate
	for _, file := range files {
		reason := ""
		if strings.TrimSpace(file.Item.Content) == "" {
			reason = "empty_content"
		} else if file.Item.Confidence < policy.MinConfidence {
			reason = "low_confidence"
		} else if file.Item.Kind == "route_health" && now.Sub(file.Item.CreatedAt) > policy.RouteHealthStaleAfter {
			reason = "stale_route_health"
		} else if file.Item.Kind != "constraint" && now.Sub(file.Item.CreatedAt) > policy.StaleAfter {
			reason = "stale_cache_item"
		} else if best[cacheDedupeKey(file.Item)].Path != file.Path {
			reason = "deduped_lower_rank"
		}
		if reason == "" {
			continue
		}
		out = append(out, QuarantineCandidate{
			ID:          file.Item.ID,
			Kind:        file.Item.Kind,
			ObjectiveID: file.Item.ObjectiveID,
			Producer:    file.Item.Producer,
			Reason:      reason,
			Confidence:  file.Item.Confidence,
			CreatedAt:   file.Item.CreatedAt,
			Path:        file.Path,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Reason == out[j].Reason {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

func promotionCandidates(files []cacheFileItem) []PromotionCandidate {
	type group struct {
		items []CacheItem
		sum   float64
	}
	groups := map[string]*group{}
	for _, file := range files {
		item := file.Item
		if item.Kind != "verified_recipe" || !item.Verified || item.Confidence < 0.75 {
			continue
		}
		key := normalizedContent(item.Content)
		if key == "" {
			continue
		}
		if groups[key] == nil {
			groups[key] = &group{}
		}
		groups[key].items = append(groups[key].items, item)
		groups[key].sum += item.Confidence
	}
	var out []PromotionCandidate
	for key, group := range groups {
		if len(group.items) < 2 {
			continue
		}
		var ids []string
		for _, item := range group.items {
			ids = append(ids, item.ID)
		}
		sort.Strings(ids)
		out = append(out, PromotionCandidate{
			Kind:       "verified_recipe",
			Count:      len(group.items),
			Confidence: group.sum / float64(len(group.items)),
			CacheIDs:   ids,
			Content:    key,
			Reason:     "repeated high-confidence verified recipe",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > 25 {
		out = out[:25]
	}
	return out
}

func (l *Lake) quarantineCacheFile(path, reason string, now time.Time) (string, error) {
	dir := filepath.Join(l.root, "quarantine", "cache", now.Format("20060102T150405Z"), reason)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(path))
	return dst, os.Rename(path, dst)
}

func normalizedContent(content string) string {
	content = strings.Join(strings.Fields(strings.ToLower(content)), " ")
	if len(content) > 500 {
		content = content[:500]
	}
	return content
}

func sortedMapKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
