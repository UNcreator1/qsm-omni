package lakebrain

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/grounding"
	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
)

func TestAnalyzeScoresNodeLakeInteraction(t *testing.T) {
	root := t.TempDir()
	q, err := lake.Open(filepath.Join(root, ".lake"))
	if err != nil {
		t.Fatal(err)
	}
	room := filepath.Join(root, ".rooms", "pos-01")
	if err := os.MkdirAll(filepath.Join(room, ".qsm_memory"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := q.PutCache(lake.CacheItem{
		ID:          "seed-constraint",
		Kind:        "constraint",
		ObjectiveID: "obj-1",
		Producer:    "executor/orchestrator",
		Content:     "Current objective request: build",
		Verified:    true,
		Confidence:  1,
		CreatedAt:   time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.PutCache(lake.CacheItem{
		ID:          "recipe-1",
		Kind:        "verified_recipe",
		ObjectiveID: "obj-1",
		PositionID:  "pos-01",
		Producer:    "executor/alpha",
		Content:     "QSM test commands passed: 2/2",
		Verified:    true,
		Confidence:  0.9,
		CreatedAt:   time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(room, ".qsm_memory", "CACHE.md"), []byte("- ID: `seed-constraint`\n- ID: `recipe-1`\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(room, "deepagents.events.jsonl"), []byte(`{"chunk":{"type":"cache_refresh","new_items":["recipe-1"]}}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(room, "evidence.json")
	if err := os.WriteFile(evidencePath, []byte(`{"cache_item_ids_observed":["seed-constraint"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := swarm.WriteRoomStatus(room, swarm.RoomStatus{
		SchemaVersion:     swarm.RoomStatusVersion,
		ObjectiveID:       "obj-1",
		PositionID:        "pos-01",
		AgentID:           "alpha",
		State:             swarm.RoomStateSucceeded,
		CacheRefreshCount: 2,
	}); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(q, swarm.RunReport{
		ObjectiveID:    "obj-1",
		RequestedNodes: 1,
		StartedNodes:   1,
		SucceededNodes: 1,
		Results: []swarm.BranchResult{{
			PositionID:   "pos-01",
			AgentID:      "alpha",
			Room:         room,
			BuildPassed:  true,
			TestPassed:   true,
			LintPassed:   true,
			EvidencePath: evidencePath,
			Citations:    []grounding.Citation{{Source: "cache_item:recipe-1"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalCacheItems != 2 || report.VerifiedCacheItems != 2 || report.AcceptedMemoryItems != 2 {
		t.Fatalf("unexpected cache counts: %#v", report)
	}
	if len(report.Nodes) != 1 || report.Nodes[0].CacheRefreshCount != 2 || report.Nodes[0].CacheItemsWritten != 1 {
		t.Fatalf("unexpected node interaction: %#v", report.Nodes)
	}
	if report.Nodes[0].QualityScore < 80 {
		t.Fatalf("expected strong node score, got %#v", report.Nodes[0])
	}
}

func TestAnalyzeSeparatesArtifactFromMemoryCitations(t *testing.T) {
	root := t.TempDir()
	q, err := lake.Open(filepath.Join(root, ".lake"))
	if err != nil {
		t.Fatal(err)
	}
	room := filepath.Join(root, ".rooms", "pos-01")
	if err := os.MkdirAll(filepath.Join(room, ".qsm_memory"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := q.PutCache(lake.CacheItem{ID: "recipe-1", Kind: "verified_recipe", ObjectiveID: "obj-2", PositionID: "pos-01", Producer: "executor/alpha", Content: "ok", Verified: true, Confidence: 1, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(room, "evidence.json")
	if err := os.WriteFile(evidencePath, []byte(`{"citations":[{"source":"lake_artifact:abc"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(q, swarm.RunReport{
		ObjectiveID:    "obj-2",
		RequestedNodes: 1,
		StartedNodes:   1,
		SucceededNodes: 1,
		Results: []swarm.BranchResult{{
			PositionID:   "pos-01",
			Room:         room,
			BuildPassed:  true,
			TestPassed:   true,
			LintPassed:   true,
			EvidencePath: evidencePath,
			Citations:    []grounding.Citation{{Source: ".lake/artifacts/abc.json"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.ArtifactCitationCoverage != 1 {
		t.Fatalf("expected artifact coverage, got %#v", report)
	}
	if report.CacheCitationCoverage != 0 || report.DecisionCitationCoverage != 0 || report.EnterpriseReady {
		t.Fatalf("artifact-only citations must not satisfy memory decision gate: %#v", report)
	}
}
