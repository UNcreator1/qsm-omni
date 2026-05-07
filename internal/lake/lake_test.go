package lake

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLakePutListAndFilter(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Put(Artifact{Phase: PhaseSynthesis, Kind: "hypothesis", Source: "alpha", Claim: "claim"}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Put(Artifact{Phase: PhaseResearch, Kind: "evidence", Source: "repo", Claim: "verified", Verified: true}); err != nil {
		t.Fatal(err)
	}
	all, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(all))
	}
	research, err := q.ByPhase(PhaseResearch)
	if err != nil {
		t.Fatal(err)
	}
	if len(research) != 1 || !research[0].Verified {
		t.Fatalf("expected one verified research artifact, got %#v", research)
	}
}

func TestLakePutIsAtomicForConcurrentSameArtifact(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	artifact := Artifact{
		Phase:   PhaseSynthesis,
		Kind:    "force_requirements_baseline",
		Source:  "qsm-planning",
		Claim:   "same deterministic artifact",
		Content: "same content",
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := q.Put(artifact); err != nil {
				t.Errorf("Put failed: %v", err)
			}
		}()
	}
	wg.Wait()
	all, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected one deterministic artifact, got %d", len(all))
	}
	data, err := os.ReadFile(filepath.Join(q.Root(), "artifacts", all[0].ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded Artifact
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("artifact JSON is corrupt: %v\n%s", err, string(data))
	}
}

func TestLakeCachePutListSummaryAndMarkdown(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.PutCache(CacheItem{
		Kind:        "failed_attempt",
		ObjectiveID: "obj-1",
		PositionID:  "pos-01",
		Producer:    "executor",
		Content:     "node hit missing asset style.css",
		Verified:    true,
		Confidence:  0.8,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.PutCache(CacheItem{
		Kind:        "verified_recipe",
		ObjectiveID: "obj-2",
		PositionID:  "pos-02",
		Producer:    "verifier",
		Content:     "static web asset check passed",
		Verified:    true,
		Confidence:  0.9,
	}); err != nil {
		t.Fatal(err)
	}
	verified := true
	items, err := q.ListCache(CacheFilter{ObjectiveID: "obj-1", Verified: &verified})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Kind != "failed_attempt" {
		t.Fatalf("unexpected cache items: %#v", items)
	}
	summary, err := q.CacheSummary("obj-1")
	if err != nil {
		t.Fatal(err)
	}
	if summary["failed_attempt"] != 1 {
		t.Fatalf("unexpected cache summary: %#v", summary)
	}
	out := filepath.Join(t.TempDir(), "CACHE.md")
	if err := q.WriteCacheMarkdown(out, CacheFilter{ObjectiveID: "obj-1"}, 10); err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("expected markdown path")
	}
}

func TestRankedCachePrioritizesConstraintsAndDedupesRouteHealth(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	items := []CacheItem{
		{Kind: "verified_recipe", ObjectiveID: "obj-1", PositionID: "pos-01", Producer: "executor", Content: "recipe", Verified: true, Confidence: 0.9, CreatedAt: now.Add(3 * time.Second)},
		{Kind: "route_health", ObjectiveID: "obj-1", Producer: "runtime", Content: "model failed", Verified: true, Confidence: 0.8, Metadata: map[string]string{"model": "m1", "status": "failed"}, CreatedAt: now},
		{Kind: "route_health", ObjectiveID: "obj-1", Producer: "runtime", Content: "model ok", Verified: true, Confidence: 0.95, Metadata: map[string]string{"model": "m1", "status": "ok"}, CreatedAt: now.Add(time.Second)},
		{Kind: "constraint", ObjectiveID: "obj-1", Producer: "executor", Content: "objective", Verified: true, Confidence: 1, CreatedAt: now.Add(2 * time.Second)},
	}
	for _, item := range items {
		if _, err := q.PutCache(item); err != nil {
			t.Fatal(err)
		}
	}
	verified := true
	ranked, err := q.RankedCache(CacheFilter{ObjectiveID: "obj-1", Verified: &verified}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked) != 3 {
		t.Fatalf("expected route-health dedupe, got %#v", ranked)
	}
	var route CacheItem
	for _, item := range ranked {
		if item.Kind == "route_health" {
			route = item
		}
	}
	if route.Content != "model ok" {
		t.Fatalf("expected latest/healthy route item, got %#v", route)
	}
	limited, err := q.RankedCache(CacheFilter{ObjectiveID: "obj-1", Verified: &verified}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0].Kind != "constraint" {
		t.Fatalf("expected constraint to survive tight limit, got %#v", limited)
	}
}
