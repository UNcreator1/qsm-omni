package grounding

import (
	"testing"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
)

func TestMapQueriesFindsExactLocalQuote(t *testing.T) {
	report := MapQueries(
		[]Candidate{{
			Source:     "wiki.md",
			SourceType: "room_memory",
			Text:       "Quantum Swarm uses deterministic collapse after build, test, and lint evidence. Other text is irrelevant.",
		}},
		[]Query{{
			Source: "product/README.md",
			Text:   "The product is approved by deterministic collapse using build, test, and lint evidence.",
		}},
		3,
	)
	if len(report.Citations) != 1 {
		t.Fatalf("expected one citation, got %#v", report)
	}
	if report.Citations[0].Quote == "" || report.Citations[0].Source != "wiki.md" {
		t.Fatalf("unexpected citation: %#v", report.Citations[0])
	}
	if report.Coverage <= 0 {
		t.Fatalf("expected positive coverage, got %#v", report)
	}
}

func TestCandidatesFromLakeUsesArtifactsAndObjectiveCache(t *testing.T) {
	q, err := lake.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Put(lake.Artifact{
		Phase:    lake.PhaseResearch,
		Kind:     "local_repo_evidence",
		Source:   "README.md",
		Claim:    "QSM has a shared cache",
		Content:  "The shared cache carries verified facts into rooms.",
		Verified: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.PutCache(lake.CacheItem{
		Kind:        "constraint",
		ObjectiveID: "obj-1",
		Producer:    "test",
		Content:     "Current objective request: build grounded memory",
		Verified:    true,
		Confidence:  1,
	}); err != nil {
		t.Fatal(err)
	}
	candidates := CandidatesFromLake(q, "obj-1")
	if len(candidates) != 2 {
		t.Fatalf("expected artifact and cache candidates, got %#v", candidates)
	}
}
