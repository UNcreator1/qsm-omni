package lakebrain

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
)

const SchemaInteractionReport = "qsm.lake_interaction_report.v1"

type Report struct {
	Schema                   string            `json:"schema"`
	ObjectiveID              string            `json:"objective_id"`
	CreatedAt                time.Time         `json:"created_at"`
	SharedCacheEnabled       bool              `json:"shared_cache_enabled"`
	TotalCacheItems          int               `json:"total_cache_items"`
	VerifiedCacheItems       int               `json:"verified_cache_items"`
	AcceptedMemoryItems      int               `json:"accepted_memory_items"`
	RejectedMemoryItems      int               `json:"rejected_memory_items"`
	CacheSummary             map[string]int    `json:"cache_summary"`
	RefreshEvents            int               `json:"refresh_events"`
	AverageRefreshPerNode    float64           `json:"average_refresh_per_node"`
	CacheWriteCoverage       float64           `json:"cache_write_coverage"`
	RefreshCoverage          float64           `json:"refresh_coverage"`
	ArtifactCitationCoverage float64           `json:"artifact_citation_coverage"`
	CacheCitationCoverage    float64           `json:"cache_citation_coverage"`
	WikiCitationCoverage     float64           `json:"wiki_citation_coverage"`
	DecisionCitationCoverage float64           `json:"decision_citation_coverage"`
	AverageNodeScore         float64           `json:"average_node_score"`
	EnterpriseReady          bool              `json:"enterprise_ready"`
	Recommendations          []string          `json:"recommendations,omitempty"`
	Nodes                    []NodeInteraction `json:"nodes"`
}

type NodeInteraction struct {
	PositionID             string   `json:"position_id"`
	AgentID                string   `json:"agent_id,omitempty"`
	AgentModel             string   `json:"agent_model,omitempty"`
	State                  string   `json:"state,omitempty"`
	CacheRefreshCount      int      `json:"cache_refresh_count"`
	FinalMemoryCacheItems  int      `json:"final_memory_cache_items"`
	NewCacheItemsObserved  int      `json:"new_cache_items_observed"`
	CacheItemsWritten      int      `json:"cache_items_written"`
	VerifiedRecipesWritten int      `json:"verified_recipes_written"`
	FailedAttemptsWritten  int      `json:"failed_attempts_written"`
	CacheCitations         int      `json:"cache_citations"`
	WikiCitations          int      `json:"wiki_citations"`
	ArtifactCitations      int      `json:"artifact_citations"`
	DecisionCitations      int      `json:"decision_citations"`
	ProductVerified        bool     `json:"product_verified"`
	QualityScore           float64  `json:"quality_score"`
	ObservedCacheIDs       []string `json:"observed_cache_ids,omitempty"`
	WrittenCacheIDs        []string `json:"written_cache_ids,omitempty"`
}

func Analyze(q *lake.Lake, report swarm.RunReport) (Report, error) {
	out := Report{
		Schema:             SchemaInteractionReport,
		ObjectiveID:        report.ObjectiveID,
		CreatedAt:          time.Now().UTC(),
		SharedCacheEnabled: len(report.CacheSummary) > 0,
		CacheSummary:       map[string]int{},
	}
	if q == nil {
		out.Recommendations = append(out.Recommendations, "Lake is unavailable; cannot measure shared memory interaction.")
		return out, nil
	}
	items, err := q.ListCache(lake.CacheFilter{ObjectiveID: report.ObjectiveID})
	if err != nil {
		return out, err
	}
	verified := true
	accepted, err := q.RankedCache(lake.CacheFilter{ObjectiveID: report.ObjectiveID, Verified: &verified}, 30)
	if err != nil {
		return out, err
	}
	out.TotalCacheItems = len(items)
	out.AcceptedMemoryItems = len(accepted)
	if out.TotalCacheItems > out.AcceptedMemoryItems {
		out.RejectedMemoryItems = out.TotalCacheItems - out.AcceptedMemoryItems
	}
	itemsByPosition := map[string][]lake.CacheItem{}
	for _, item := range items {
		out.CacheSummary[item.Kind]++
		if item.Verified {
			out.VerifiedCacheItems++
		}
		if item.PositionID != "" {
			itemsByPosition[item.PositionID] = append(itemsByPosition[item.PositionID], item)
		}
	}
	statuses := roomStatuses(report)
	var nodesWithWrites, nodesWithRefresh, nodesWithCacheCitation, nodesWithWikiCitation, nodesWithArtifactCitation, nodesWithDecisionCitation int
	for _, result := range report.Results {
		status := statuses[result.PositionID]
		node := NodeInteraction{
			PositionID:        result.PositionID,
			AgentID:           result.AgentID,
			AgentModel:        result.AgentModel,
			State:             status.State,
			CacheRefreshCount: status.CacheRefreshCount,
			ProductVerified:   result.BuildPassed && result.TestPassed && result.LintPassed && result.Error == "",
		}
		if node.State == "" {
			if node.ProductVerified {
				node.State = swarm.RoomStateSucceeded
			} else {
				node.State = swarm.RoomStateFailed
			}
		}
		node.ObservedCacheIDs = sortedUnique(append(cacheIDsFromMarkdown(filepath.Join(result.Room, ".qsm_memory", "CACHE.md")), cacheIDsFromEvents(filepath.Join(result.Room, "deepagents.events.jsonl"))...))
		node.FinalMemoryCacheItems = len(cacheIDsFromMarkdown(filepath.Join(result.Room, ".qsm_memory", "CACHE.md")))
		node.NewCacheItemsObserved = len(cacheIDsFromEvents(filepath.Join(result.Room, "deepagents.events.jsonl")))
		for _, item := range itemsByPosition[result.PositionID] {
			node.CacheItemsWritten++
			node.WrittenCacheIDs = append(node.WrittenCacheIDs, item.ID)
			switch item.Kind {
			case "verified_recipe":
				node.VerifiedRecipesWritten++
			case "failed_attempt":
				node.FailedAttemptsWritten++
			}
		}
		node.WrittenCacheIDs = sortedUnique(node.WrittenCacheIDs)
		for _, citation := range result.Citations {
			switch {
			case strings.HasPrefix(citation.Source, "cache_item:"):
				node.CacheCitations++
			case strings.HasPrefix(citation.Source, "wiki_item:"):
				node.WikiCitations++
			case strings.HasPrefix(citation.Source, "lake_artifact:") || strings.Contains(citation.Source, ".lake/artifacts/"):
				node.ArtifactCitations++
			}
		}
		node.CacheCitations += len(cacheIDsFromMetadata(result.Metadata))
		node.CacheCitations += len(cacheIDsFromEvidence(result.EvidencePath))
		node.WikiCitations += len(wikiIDsFromMetadata(result.Metadata))
		node.WikiCitations += len(wikiIDsFromEvidence(result.EvidencePath))
		node.DecisionCitations = node.CacheCitations + node.WikiCitations
		node.QualityScore = nodeScore(node)
		if node.CacheItemsWritten > 0 {
			nodesWithWrites++
		}
		if node.CacheRefreshCount > 0 {
			nodesWithRefresh++
		}
		if node.CacheCitations > 0 {
			nodesWithCacheCitation++
		}
		if node.WikiCitations > 0 {
			nodesWithWikiCitation++
		}
		if node.ArtifactCitations > 0 {
			nodesWithArtifactCitation++
		}
		if node.DecisionCitations > 0 {
			nodesWithDecisionCitation++
		}
		out.RefreshEvents += node.CacheRefreshCount
		out.AverageNodeScore += node.QualityScore
		out.Nodes = append(out.Nodes, node)
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].PositionID < out.Nodes[j].PositionID })
	nodeCount := len(out.Nodes)
	if nodeCount > 0 {
		out.AverageRefreshPerNode = float64(out.RefreshEvents) / float64(nodeCount)
		out.CacheWriteCoverage = float64(nodesWithWrites) / float64(nodeCount)
		out.RefreshCoverage = float64(nodesWithRefresh) / float64(nodeCount)
		out.ArtifactCitationCoverage = float64(nodesWithArtifactCitation) / float64(nodeCount)
		out.CacheCitationCoverage = float64(nodesWithCacheCitation) / float64(nodeCount)
		out.WikiCitationCoverage = float64(nodesWithWikiCitation) / float64(nodeCount)
		out.DecisionCitationCoverage = float64(nodesWithDecisionCitation) / float64(nodeCount)
		out.AverageNodeScore = out.AverageNodeScore / float64(nodeCount)
	}
	out.EnterpriseReady = out.RefreshCoverage >= 1 && out.CacheWriteCoverage >= 1 && out.DecisionCitationCoverage >= 0.7 && out.AverageNodeScore >= 75 && out.TotalCacheItems > 0
	out.Recommendations = recommendations(out)
	return out, nil
}

func Write(root string, report Report) error {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	stateDir := filepath.Join(root, ".state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stateDir, "lake_interaction_score.json"), data, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, "lake_interaction_score.md"), []byte(Markdown(report)), 0644)
}

func Markdown(report Report) string {
	var b strings.Builder
	b.WriteString("# QSM Lake Interaction Score\n\n")
	b.WriteString(fmt.Sprintf("- Objective: `%s`\n", report.ObjectiveID))
	b.WriteString(fmt.Sprintf("- Enterprise ready: `%v`\n", report.EnterpriseReady))
	b.WriteString(fmt.Sprintf("- Cache items: `%d` total, `%d` verified, `%d` accepted into memory, `%d` rejected by rank/limit\n", report.TotalCacheItems, report.VerifiedCacheItems, report.AcceptedMemoryItems, report.RejectedMemoryItems))
	b.WriteString(fmt.Sprintf("- Refresh: `%d` events, `%.2f` average per node, `%.0f%%` node coverage\n", report.RefreshEvents, report.AverageRefreshPerNode, report.RefreshCoverage*100))
	b.WriteString(fmt.Sprintf("- Write coverage: `%.0f%%`\n", report.CacheWriteCoverage*100))
	b.WriteString(fmt.Sprintf("- Artifact citation coverage: `%.0f%%`\n", report.ArtifactCitationCoverage*100))
	b.WriteString(fmt.Sprintf("- Cache citation coverage: `%.0f%%`\n", report.CacheCitationCoverage*100))
	b.WriteString(fmt.Sprintf("- Wiki citation coverage: `%.0f%%`\n", report.WikiCitationCoverage*100))
	b.WriteString(fmt.Sprintf("- Decision citation coverage: `%.0f%%`\n", report.DecisionCitationCoverage*100))
	b.WriteString(fmt.Sprintf("- Average node score: `%.1f`\n\n", report.AverageNodeScore))
	if len(report.Recommendations) > 0 {
		b.WriteString("## Recommendations\n\n")
		for _, rec := range report.Recommendations {
			b.WriteString("- " + rec + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Nodes\n\n")
	b.WriteString("| Position | State | Refresh | Final Cache | New Seen | Written | Artifact | Cache | Wiki | Decision | Score |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, node := range report.Nodes {
		b.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d | %d | %d | %d | %d | %d | %.1f |\n",
			node.PositionID, node.State, node.CacheRefreshCount, node.FinalMemoryCacheItems, node.NewCacheItemsObserved, node.CacheItemsWritten, node.ArtifactCitations, node.CacheCitations, node.WikiCitations, node.DecisionCitations, node.QualityScore))
	}
	return b.String()
}

func roomStatuses(report swarm.RunReport) map[string]swarm.RoomStatus {
	statuses := map[string]swarm.RoomStatus{}
	for _, result := range report.Results {
		status, err := swarm.ReadRoomStatus(result.Room)
		if err == nil {
			statuses[result.PositionID] = status
		}
	}
	return statuses
}

func nodeScore(node NodeInteraction) float64 {
	var score float64
	if node.CacheRefreshCount > 0 {
		score += 25
	}
	if node.FinalMemoryCacheItems > 0 {
		score += 20
	}
	if node.NewCacheItemsObserved > 0 {
		score += 10
	}
	if node.CacheItemsWritten > 0 {
		score += 25
	}
	if node.CacheCitations > 0 {
		score += 10
	}
	if node.ProductVerified {
		score += 10
	}
	return score
}

func recommendations(report Report) []string {
	var out []string
	if report.TotalCacheItems == 0 {
		out = append(out, "No objective-scoped cache items were written; seed constraints and require verified recipe/failure writes.")
	}
	if report.RefreshCoverage < 1 {
		out = append(out, "Some nodes did not refresh cache; make live cache mandatory for every real harness.")
	}
	if report.CacheWriteCoverage < 1 {
		out = append(out, "Some nodes wrote no cache items; require every node to publish a verified recipe or failed_attempt.")
	}
	if report.DecisionCitationCoverage < 0.7 {
		out = append(out, "Few outputs cite cache/wiki memory items; require evidence.json to cite cache_item or wiki_item IDs used for factual decisions.")
	}
	if report.RejectedMemoryItems > 0 {
		out = append(out, "Cache ranking is pruning items from prompt memory; review rejected items periodically and archive stale/noisy facts.")
	}
	if report.AverageNodeScore < 75 {
		out = append(out, "Average lake interaction score is below enterprise target 75; improve attribution and cache usage.")
	}
	if len(out) == 0 {
		out = append(out, "Lake interaction meets the current local enterprise gate; next step is repeated-cycle measurement.")
	}
	return out
}

var cacheIDPattern = regexp.MustCompile("- ID: `([^`]+)`")

func cacheIDsFromMarkdown(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	matches := cacheIDPattern.FindAllStringSubmatch(string(data), -1)
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			ids = append(ids, match[1])
		}
	}
	return sortedUnique(ids)
}

func cacheIDsFromEvents(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var ids []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var event struct {
			Chunk struct {
				Type     string   `json:"type"`
				NewItems []string `json:"new_items"`
			} `json:"chunk"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Chunk.Type == "cache_refresh" {
			ids = append(ids, event.Chunk.NewItems...)
		}
	}
	return sortedUnique(ids)
}

func cacheIDsFromEvidence(path string) []string {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	ids := append(anyStringSlice(raw["cache_item_ids_used"]), anyStringSlice(raw["cache_items_used"])...)
	ids = append(ids, anyStringSlice(raw["cache_item_ids_observed"])...)
	ids = append(ids, prefixedIDsFromRaw(raw["memory_citations"], "cache_item:")...)
	ids = append(ids, prefixedIDsFromRaw(raw["citations"], "cache_item:")...)
	return sortedUnique(ids)
}

func cacheIDsFromMetadata(metadata map[string]any) []string {
	if len(metadata) == 0 {
		return nil
	}
	ids := append(anyStringSlice(metadata["cache_item_ids_used"]), anyStringSlice(metadata["cache_items_used"])...)
	ids = append(ids, anyStringSlice(metadata["cache_item_ids_observed"])...)
	ids = append(ids, prefixedIDsFromRaw(metadata["memory_citations"], "cache_item:")...)
	return sortedUnique(ids)
}

func wikiIDsFromEvidence(path string) []string {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	ids := append(anyStringSlice(raw["wiki_item_ids_used"]), anyStringSlice(raw["wiki_items_used"])...)
	ids = append(ids, prefixedIDsFromRaw(raw["memory_citations"], "wiki_item:")...)
	ids = append(ids, prefixedIDsFromRaw(raw["citations"], "wiki_item:")...)
	return sortedUnique(ids)
}

func wikiIDsFromMetadata(metadata map[string]any) []string {
	if len(metadata) == 0 {
		return nil
	}
	ids := append(anyStringSlice(metadata["wiki_item_ids_used"]), anyStringSlice(metadata["wiki_items_used"])...)
	ids = append(ids, prefixedIDsFromRaw(metadata["memory_citations"], "wiki_item:")...)
	return sortedUnique(ids)
}

func prefixedIDsFromRaw(value any, prefix string) []string {
	var out []string
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			out = append(out, prefixedIDFromAny(item, prefix))
		}
	case []string:
		for _, item := range typed {
			out = append(out, prefixedIDFromText(item, prefix))
		}
	case map[string]any:
		out = append(out, prefixedIDFromAny(typed, prefix))
	}
	return sortedUnique(out)
}

func prefixedIDFromAny(value any, prefix string) string {
	if m, ok := value.(map[string]any); ok {
		for _, key := range []string{"source", "id", "citation_id"} {
			if id := prefixedIDFromText(fmt.Sprint(m[key]), prefix); id != "" {
				return id
			}
		}
		return ""
	}
	return prefixedIDFromText(fmt.Sprint(value), prefix)
}

func prefixedIDFromText(value, prefix string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(value, prefix))
	}
	return ""
}

func anyStringSlice(value any) []string {
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		return typed
	default:
		return nil
	}
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
