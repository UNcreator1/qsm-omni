package collapse

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	"github.com/nemoclaws/quantum-swarm-v3/internal/productmanifest"
	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
)

func TestCollapseSelectsAuditedWinner(t *testing.T) {
	q, err := lake.Open(filepath.Join(t.TempDir(), ".lake"))
	if err != nil {
		t.Fatal(err)
	}
	product := filepath.Join(t.TempDir(), "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "README.md"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := productmanifest.Write(filepath.Join(product, "qsm_project_manifest.v1.json"), productmanifest.Manifest{
		Version:           productmanifest.Schema,
		ProductKind:       "data-transform",
		ExpectedArtifacts: []string{"README.md"},
		TestCommands:      []string{"true"},
		MemoryCitations:   []string{"wiki_item:test"},
	}); err != nil {
		t.Fatal(err)
	}
	results := []swarm.BranchResult{
		{PositionID: "pos-01", BuildPassed: true, TestPassed: true, LintPassed: true, Score: 0.7, ProductPath: product},
		{PositionID: "pos-02", BuildPassed: true, TestPassed: false, LintPassed: true, Score: 0.9},
	}
	findings := Audit(results)
	verdict, err := ConsensusEngine{Lake: q}.Collapse(results, findings)
	if err != nil {
		t.Fatal(err)
	}
	if !verdict.Approved {
		t.Fatalf("expected approved verdict: %#v", verdict)
	}
	if verdict.Winner.PositionID != "pos-01" {
		t.Fatalf("expected pos-01 winner, got %s", verdict.Winner.PositionID)
	}
}
