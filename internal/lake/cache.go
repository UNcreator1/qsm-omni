package lake

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CacheItem struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"`
	ObjectiveID string            `json:"objective_id,omitempty"`
	PositionID  string            `json:"position_id,omitempty"`
	Producer    string            `json:"producer"`
	Content     string            `json:"content"`
	Verified    bool              `json:"verified"`
	Confidence  float64           `json:"confidence"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

type CacheFilter struct {
	ObjectiveID string
	Kinds       []string
	Verified    *bool
	Since       time.Time
	Before      time.Time
}

var cacheKindPriority = map[string]int{
	"constraint":        100,
	"route_health":      95,
	"failed_attempt":    90,
	"verified_recipe":   85,
	"dependency_note":   75,
	"score_signal":      70,
	"rate_limit_signal": 65,
}

func (l *Lake) PutCache(item CacheItem) (CacheItem, error) {
	if strings.TrimSpace(item.Kind) == "" {
		return item, errors.New("cache kind is required")
	}
	if strings.TrimSpace(item.Producer) == "" {
		return item, errors.New("cache producer is required")
	}
	if strings.TrimSpace(item.Content) == "" {
		return item, errors.New("cache content is required")
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	if item.Metadata == nil {
		item.Metadata = map[string]string{}
	}
	if item.ID == "" {
		sum := sha256.Sum256([]byte(item.Kind + "\x00" + item.ObjectiveID + "\x00" + item.PositionID + "\x00" + item.Producer + "\x00" + item.Content + "\x00" + item.CreatedAt.Format(time.RFC3339Nano)))
		item.ID = hex.EncodeToString(sum[:])[:16]
	}
	if err := os.MkdirAll(l.cacheDir(), 0755); err != nil {
		return item, err
	}
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return item, err
	}
	name := fmt.Sprintf("%s-%s.json", item.CreatedAt.Format("20060102T150405.000000000Z"), item.ID)
	return item, os.WriteFile(filepath.Join(l.cacheDir(), name), data, 0644)
}

func (l *Lake) ListCache(filter CacheFilter) ([]CacheItem, error) {
	entries, err := os.ReadDir(l.cacheDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	kindSet := map[string]bool{}
	for _, kind := range filter.Kinds {
		if kind != "" {
			kindSet[kind] = true
		}
	}
	var out []CacheItem
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(l.cacheDir(), entry.Name()))
		if err != nil {
			return nil, err
		}
		var item CacheItem
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if filter.ObjectiveID != "" && item.ObjectiveID != filter.ObjectiveID {
			continue
		}
		if len(kindSet) > 0 && !kindSet[item.Kind] {
			continue
		}
		if filter.Verified != nil && item.Verified != *filter.Verified {
			continue
		}
		if !filter.Since.IsZero() && item.CreatedAt.Before(filter.Since) {
			continue
		}
		if !filter.Before.IsZero() && !item.CreatedAt.Before(filter.Before) {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (l *Lake) CacheSummary(objectiveID string) (map[string]int, error) {
	items, err := l.ListCache(CacheFilter{ObjectiveID: objectiveID})
	if err != nil {
		return nil, err
	}
	summary := map[string]int{}
	for _, item := range items {
		summary[item.Kind]++
	}
	return summary, nil
}

func (l *Lake) WriteCacheMarkdown(path string, filter CacheFilter, limit int) error {
	items, err := l.RankedCache(filter, limit)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# QSM Shared Cache\n\n")
	b.WriteString("Only verified facts, negative lessons, and scheduler signals should be used here. Do not copy sibling branch products or strategies.\n\n")
	if len(items) == 0 {
		b.WriteString("No cache items are available for this objective yet.\n")
		return os.WriteFile(path, []byte(b.String()), 0644)
	}
	for _, item := range items {
		status := "unverified"
		if item.Verified {
			status = "verified"
		}
		b.WriteString(fmt.Sprintf("## %s / %s / %s\n\n", item.Kind, item.Producer, item.PositionID))
		b.WriteString(fmt.Sprintf("- ID: `%s`\n- Status: `%s`\n- Confidence: `%.2f`\n- Created: `%s`\n", item.ID, status, item.Confidence, item.CreatedAt.Format(time.RFC3339)))
		if len(item.Metadata) > 0 {
			b.WriteString("- Metadata:\n")
			keys := make([]string, 0, len(item.Metadata))
			for key := range item.Metadata {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				b.WriteString(fmt.Sprintf("  - %s: %s\n", key, item.Metadata[key]))
			}
		}
		b.WriteString("\n```text\n")
		b.WriteString(strings.TrimSpace(item.Content))
		b.WriteString("\n```\n\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func (l *Lake) RankedCache(filter CacheFilter, limit int) ([]CacheItem, error) {
	items, err := l.ListCache(filter)
	if err != nil {
		return nil, err
	}
	items = dedupeCacheItems(items)
	sort.SliceStable(items, func(i, j int) bool {
		left := cacheRank(items[i])
		right := cacheRank(items[j])
		if left == right {
			if items[i].CreatedAt.Equal(items[j].CreatedAt) {
				return items[i].ID < items[j].ID
			}
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return left > right
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func dedupeCacheItems(items []CacheItem) []CacheItem {
	kept := map[string]CacheItem{}
	for _, item := range items {
		key := cacheDedupeKey(item)
		prev, ok := kept[key]
		if !ok || cacheRank(item) > cacheRank(prev) || (cacheRank(item) == cacheRank(prev) && item.CreatedAt.After(prev.CreatedAt)) {
			kept[key] = item
		}
	}
	out := make([]CacheItem, 0, len(kept))
	for _, item := range kept {
		out = append(out, item)
	}
	return out
}

func cacheDedupeKey(item CacheItem) string {
	if item.Kind == "route_health" {
		model := strings.TrimSpace(item.Metadata["model"])
		if model != "" {
			return item.Kind + "\x00" + item.ObjectiveID + "\x00" + model
		}
	}
	content := strings.Join(strings.Fields(strings.ToLower(item.Content)), " ")
	if len(content) > 240 {
		content = content[:240]
	}
	return item.Kind + "\x00" + item.ObjectiveID + "\x00" + item.PositionID + "\x00" + item.Producer + "\x00" + content
}

func cacheRank(item CacheItem) float64 {
	score := float64(cacheKindPriority[item.Kind])
	score += item.Confidence * 10
	if item.Verified {
		score += 5
	}
	if item.Kind == "route_health" && strings.EqualFold(item.Metadata["status"], "ok") {
		score += 3
	}
	if item.Kind == "failed_attempt" {
		score += 2
	}
	return score
}

func (l *Lake) cacheDir() string {
	return filepath.Join(l.root, "cache")
}
