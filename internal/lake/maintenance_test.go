package lake

import (
	"os"
	"testing"
	"time"
)

func TestMaintainCacheReportsDuplicatesAndPromotions(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	items := []CacheItem{
		{Kind: "verified_recipe", ObjectiveID: "obj-1", PositionID: "pos-01", Producer: "alpha", Content: "Use a real test before evidence.", Verified: true, Confidence: 0.8, CreatedAt: now.Add(-3 * time.Hour)},
		{Kind: "verified_recipe", ObjectiveID: "obj-1", PositionID: "pos-01", Producer: "alpha", Content: "Use a real test before evidence.", Verified: true, Confidence: 0.9, CreatedAt: now.Add(-2 * time.Hour)},
		{Kind: "verified_recipe", ObjectiveID: "obj-2", PositionID: "pos-02", Producer: "beta", Content: "Use a real test before evidence.", Verified: true, Confidence: 0.85, CreatedAt: now.Add(-1 * time.Hour)},
		{Kind: "failed_attempt", ObjectiveID: "obj-1", PositionID: "pos-03", Producer: "gamma", Content: "weak", Verified: true, Confidence: 0.2, CreatedAt: now},
	}
	for _, item := range items {
		if _, err := q.PutCache(item); err != nil {
			t.Fatal(err)
		}
	}
	report, err := q.MaintainCache(MaintenancePolicy{MinConfidence: 0.45})
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalCacheItems != 4 {
		t.Fatalf("unexpected total: %#v", report)
	}
	if report.QuarantineCount != 2 {
		t.Fatalf("expected duplicate and low-confidence candidates: %#v", report.QuarantineCandidates)
	}
	if len(report.PromotionCandidates) != 1 || report.PromotionCandidates[0].Count != 3 {
		t.Fatalf("expected repeated recipe promotion: %#v", report.PromotionCandidates)
	}
}

func TestMaintainCacheApplyMovesToQuarantine(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.PutCache(CacheItem{
		Kind:       "failed_attempt",
		Producer:   "test",
		Content:    "low confidence",
		Verified:   true,
		Confidence: 0.1,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	report, err := q.MaintainCache(MaintenancePolicy{Apply: true, MinConfidence: 0.45})
	if err != nil {
		t.Fatal(err)
	}
	if report.QuarantineCount != 1 || len(report.Actions) != 1 {
		t.Fatalf("expected one quarantine action: %#v", report)
	}
	if _, err := os.Stat(report.QuarantineCandidates[0].QuarantinePath); err != nil {
		t.Fatalf("expected quarantine file: %v", err)
	}
	items, err := q.ListCache(CacheFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected active cache to be empty after quarantine, got %#v", items)
	}
}
