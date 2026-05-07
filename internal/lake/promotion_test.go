package lake

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPromoteCacheRejectsLowSignalAndPromotesReusableRecipe(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	reusable := "When building static web deliverables, create a behavioral smoke test that verifies every script and stylesheet referenced by index.html exists before writing evidence."
	for i := 0; i < 3; i++ {
		if _, err := q.PutCache(CacheItem{
			Kind:        "verified_recipe",
			ObjectiveID: "obj",
			PositionID:  "pos",
			Producer:    "test",
			Content:     reusable,
			Verified:    true,
			Confidence:  0.85,
			CreatedAt:   now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 4; i++ {
		if _, err := q.PutCache(CacheItem{
			Kind:       "verified_recipe",
			Producer:   "test",
			Content:    "product directory is non-empty",
			Verified:   true,
			Confidence: 0.9,
			CreatedAt:  now.Add(time.Duration(i+10) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := q.PromoteCache(PromotionPolicy{MinRepeat: 3, MinConfidence: 0.75})
	if err != nil {
		t.Fatal(err)
	}
	if report.Promoted != 1 {
		t.Fatalf("expected one promotion, got %#v", report)
	}
	if report.Rejected == 0 {
		t.Fatalf("expected low-signal rejection, got %#v", report)
	}
	if len(report.CuratedFiles) != 0 {
		t.Fatalf("dry run should not write curated files, got %v", report.CuratedFiles)
	}
}

func TestPromoteCacheApplyWritesCuratedFilesAndArtifacts(t *testing.T) {
	root := t.TempDir()
	q, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	content := "For CLI products, include a deterministic command-line smoke test with both expected output and non-zero failure behavior before marking test_passed true."
	for i := 0; i < 3; i++ {
		if _, err := q.PutCache(CacheItem{
			Kind:       "verified_recipe",
			Producer:   "test",
			Content:    content,
			Verified:   true,
			Confidence: 0.88,
			CreatedAt:  time.Now().UTC().Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := q.PromoteCache(PromotionPolicy{Apply: true, MinRepeat: 3, MinConfidence: 0.75})
	if err != nil {
		t.Fatal(err)
	}
	if report.Promoted != 1 || len(report.ArtifactsWritten) != 1 {
		t.Fatalf("expected one written artifact: %#v", report)
	}
	for _, path := range []string{
		filepath.Join(root, "curated", "best_practices.json"),
		filepath.Join(root, "curated", "best_practices.md"),
		filepath.Join(root, "artifacts", report.ArtifactsWritten[0]+".json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file %s: %v", path, err)
		}
	}
}

func TestPromoteCacheApplyDoesNotAdvertiseEmptyCuratedFiles(t *testing.T) {
	root := t.TempDir()
	q, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := q.PutCache(CacheItem{
			Kind:       "verified_recipe",
			Producer:   "test",
			Content:    "product directory is non-empty",
			Verified:   true,
			Confidence: 0.92,
			CreatedAt:  time.Now().UTC().Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := q.PromoteCache(PromotionPolicy{Apply: true, MinRepeat: 3, MinConfidence: 0.75})
	if err != nil {
		t.Fatal(err)
	}
	if report.Promoted != 0 {
		t.Fatalf("expected no promotions, got %#v", report)
	}
	if len(report.CuratedFiles) != 0 {
		t.Fatalf("expected no curated files for empty promotion set, got %v", report.CuratedFiles)
	}
	if _, err := os.Stat(filepath.Join(root, "curated", "best_practices.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no empty curated index write, stat err=%v", err)
	}
}
