package costing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
)

func TestAnalyzeUsesObservedMetadataAndRates(t *testing.T) {
	t.Setenv("QSM_COST_USD_PER_1M_INPUT_OC_TEST_MODEL", "2")
	t.Setenv("QSM_COST_USD_PER_1M_OUTPUT_OC_TEST_MODEL", "8")
	report := Analyze(swarm.RunReport{
		ObjectiveID: "obj-1",
		Results: []swarm.BranchResult{{
			PositionID:  "pos-01",
			AgentModel:  "oc/test-model",
			BuildPassed: true,
			TestPassed:  true,
			LintPassed:  true,
			Metadata: map[string]any{
				"input_tokens":  1000000,
				"output_tokens": 500000,
			},
		}},
	})
	if report.TotalTokens != 1500000 {
		t.Fatalf("unexpected token total: %#v", report)
	}
	if report.EstimatedUSD != 6 {
		t.Fatalf("unexpected cost: %#v", report)
	}
	if !report.Nodes[0].ObservedUsage || report.Nodes[0].EstimationMethod != "provider_usage_metadata" {
		t.Fatalf("expected observed usage metadata, got %#v", report.Nodes[0])
	}
}

func TestAnalyzeEstimatesFromRoomFiles(t *testing.T) {
	root := t.TempDir()
	room := filepath.Join(root, "pos-01")
	if err := os.MkdirAll(filepath.Join(room, ".qsm_memory"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(room, ".qsm_memory", "AGENTS.md"), []byte("12345678"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(room, "evidence.json"), []byte("123456789012"), 0644); err != nil {
		t.Fatal(err)
	}
	report := Analyze(swarm.RunReport{
		ObjectiveID: "obj-2",
		Results: []swarm.BranchResult{{
			PositionID:   "pos-01",
			AgentModel:   "unknown",
			Room:         room,
			EvidencePath: filepath.Join(room, "evidence.json"),
		}},
	})
	if report.InputTokens != 2 || report.OutputTokens != 3 || report.TotalTokens != 5 {
		t.Fatalf("unexpected estimate: %#v", report)
	}
}
