package capacity

import "testing"

func TestEstimate16GB10CPU(t *testing.T) {
	plan := Estimate(Hardware{MemoryBytes: 16 * 1024 * MiB, LogicalCPU: 10}, DefaultProfile())
	if plan.PerNodeMiB != 832 {
		t.Fatalf("unexpected per-node budget: %d", plan.PerNodeMiB)
	}
	if plan.RecommendedNodes != 7 {
		t.Fatalf("expected 7 recommended nodes, got %d (%s)", plan.RecommendedNodes, plan.Summary())
	}
	if plan.RecommendedPositions != plan.RecommendedNodes {
		t.Fatalf("positions should follow node recommendation")
	}
}

func TestEstimateAlwaysAtLeastOneNode(t *testing.T) {
	plan := Estimate(Hardware{MemoryBytes: 512 * MiB, LogicalCPU: 1}, DefaultProfile())
	if plan.RecommendedNodes != 1 {
		t.Fatalf("expected fallback of 1 node, got %d", plan.RecommendedNodes)
	}
}
