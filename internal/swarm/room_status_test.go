package swarm

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoomStatusWriteReadAndResult(t *testing.T) {
	room := t.TempDir()
	status := NewRoomStatus(
		Objective{ID: "obj-1"},
		Position{ID: "pos-01", Room: room},
		Agent{ID: "alpha", Provider: "oc", Model: "ling"},
		"langchain",
	)
	if err := WriteRoomStatus(room, status); err != nil {
		t.Fatal(err)
	}

	MarkRoomCacheRefresh(room, 1)
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "README.md"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(room, "evidence.json")
	if err := os.WriteFile(evidence, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	MarkRoomResult(room, BranchResult{
		PositionID:   "pos-01",
		AgentID:      "alpha",
		AgentModel:   "oc/ling",
		Room:         room,
		BuildPassed:  true,
		TestPassed:   true,
		LintPassed:   true,
		Score:        0.9,
		EvidencePath: evidence,
		ProductPath:  product,
		CompletedAt:  time.Now().UTC(),
	})

	got, err := ReadRoomStatus(room)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != RoomStateSucceeded || got.CacheRefreshCount != 1 || !got.ProductReady || !got.EvidenceReady {
		t.Fatalf("unexpected status: %#v", got)
	}
	if got.AgentModel != "oc/ling" || got.Phase != "complete" {
		t.Fatalf("unexpected final metadata: %#v", got)
	}
}

func TestAgentRouteLabel(t *testing.T) {
	cases := []struct {
		agent Agent
		want  string
	}{
		{agent: Agent{Provider: "oc", Model: "m"}, want: "oc/m"},
		{agent: Agent{Provider: "", Model: "deepseek-chat"}, want: "deepseek-chat"},
		{agent: Agent{Provider: "oc", Model: "oc/m"}, want: "oc/m"},
	}
	for _, tc := range cases {
		if got := AgentRouteLabel(tc.agent); got != tc.want {
			t.Fatalf("AgentRouteLabel(%#v) = %q, want %q", tc.agent, got, tc.want)
		}
	}
}
